// @title           Notification System API
// @version         1.0
// @description     Event-Driven Notification System with hexagonal architecture
// @termsOfService  http://swagger.io/terms/

// @contact.name   Insider Engineering
// @contact.email  mete.vahi@insider.com

// @host      localhost:8080
// @BasePath  /

// @schemes   http https
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	appNotification "github.com/insider/notification-system/internal/application/notification"
	httpAdapter "github.com/insider/notification-system/internal/adapters/http"
	pgAdapter "github.com/insider/notification-system/internal/adapters/postgres"
	redisAdapter "github.com/insider/notification-system/internal/adapters/redis"
	webhookAdapter "github.com/insider/notification-system/internal/adapters/webhook"
	"github.com/insider/notification-system/internal/worker"
	"github.com/insider/notification-system/pkg/config"
	"github.com/insider/notification-system/pkg/logger"
	"github.com/insider/notification-system/pkg/metrics"
)

func main() {
	cfg := config.Load()

	// Initialize logger.
	log, err := logger.New(os.Getenv("ENV") == "development")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync() //nolint:errcheck
	zap.ReplaceGlobals(log)

	log.Info("starting notification service",
		zap.String("port", cfg.Server.Port),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = logger.WithLogger(ctx, log)

	// Run database migrations.
	if err := runMigrations(cfg.Database.DSN, log); err != nil {
		log.Warn("migration warning (non-fatal)", zap.Error(err))
	}

	// Initialize PostgreSQL connection pool.
	pool, err := pgAdapter.NewPool(ctx, cfg.Database.DSN)
	if err != nil {
		log.Fatal("failed to connect to postgres", zap.Error(err))
	}
	defer pool.Close()
	log.Info("connected to postgresql")

	// Initialize Redis client.
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal("failed to connect to redis", zap.Error(err))
	}
	defer redisClient.Close()
	log.Info("connected to redis")

	// Initialize Prometheus metrics.
	m := metrics.New()

	// Wire up adapters.
	repo := pgAdapter.New(pool)
	queue := redisAdapter.NewQueue(redisClient)
	rateLimiter := redisAdapter.NewRateLimiter(redisClient)
	provider := webhookAdapter.New(cfg.Webhook.URL, cfg.Webhook.Timeout)

	// Initialize application service.
	svc := appNotification.NewService(repo, queue, provider, rateLimiter, m)

	// Initialize HTTP handler and router.
	handler := httpAdapter.NewHandler(svc, log)
	handler.StartHub()
	router := httpAdapter.NewRouter(handler, log, m)

	// Start worker processor.
	processor := worker.NewProcessor(
		svc, queue, repo,
		cfg.Worker.WorkersPerChannel,
		cfg.Worker.RetryBaseDelay,
		cfg.Worker.MaxRetries,
		m, log,
	)
	go processor.Start(ctx)

	// Start scheduler.
	scheduler := worker.NewScheduler(repo, queue, log)
	go scheduler.Start(ctx)

	// Start HTTP server.
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		log.Info("http server listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("http server error", zap.Error(err))
		}
	}()

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("server forced to shutdown", zap.Error(err))
	}

	log.Info("server exited")
}

func runMigrations(dsn string, log *zap.Logger) error {
	m, err := migrate.New("file://migrations", dsn)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			log.Info("database schema is up to date")
			return nil
		}
		return fmt.Errorf("run migrations: %w", err)
	}

	log.Info("database migrations applied successfully")
	return nil
}
