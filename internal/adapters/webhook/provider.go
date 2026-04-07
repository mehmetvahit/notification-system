package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	domain "github.com/insider/notification-system/internal/domain/notification"
)

// sendRequest is the JSON payload sent to the webhook endpoint.
type sendRequest struct {
	To      string `json:"to"`
	Channel string `json:"channel"`
	Content string `json:"content"`
}

// sendResponse is the expected JSON response from the webhook endpoint.
type sendResponse struct {
	MessageID string `json:"messageId"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// Provider implements domain.Provider by posting to a configurable webhook URL.
type Provider struct {
	url    string
	client *http.Client
}

// New creates a new webhook provider with the given endpoint URL and timeout.
func New(webhookURL string, timeout time.Duration) *Provider {
	return &Provider{
		url: webhookURL,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Send delivers a notification by POSTing to the configured webhook URL.
// It expects a 202 Accepted response with a JSON body containing messageId.
func (p *Provider) Send(ctx context.Context, n *domain.Notification) (string, error) {
	payload := sendRequest{
		To:      n.Recipient,
		Channel: string(n.Channel),
		Content: n.Content,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Notification-ID", n.ID)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read webhook response: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("webhook returned unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var webhookResp sendResponse
	if err := json.Unmarshal(respBody, &webhookResp); err != nil {
		return "", fmt.Errorf("unmarshal webhook response: %w", err)
	}

	if webhookResp.MessageID == "" {
		return "", fmt.Errorf("webhook response missing messageId")
	}

	return webhookResp.MessageID, nil
}
