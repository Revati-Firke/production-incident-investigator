package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Revati-Firke/production-incident-investigator/internal/integrations"
)

// Client sends Slack notifications or returns mock results.
type Client interface {
	Provider() string
	PostMessage(ctx context.Context, channel, message string) (*integrations.SlackMessage, error)
}

// MockClient simulates Slack without credentials.
type MockClient struct{}

func NewMock() *MockClient { return &MockClient{} }

func (c *MockClient) Provider() string { return integrations.ProviderMock }

func (c *MockClient) PostMessage(_ context.Context, channel, _ string) (*integrations.SlackMessage, error) {
	if channel == "" {
		channel = "#incidents"
	}
	return &integrations.SlackMessage{Channel: channel, MessageID: "msg-mock-001", OK: true}, nil
}

// HTTPClient uses Slack Web API chat.postMessage.
type HTTPClient struct {
	token     string
	defaultCh string
	baseURL   string
	client    *http.Client
}

// Config configures Slack HTTP client.
type Config struct {
	Token          string
	DefaultChannel string
	BaseURL        string
	Timeout        time.Duration
}

// NewHTTP creates a Slack HTTP client.
func NewHTTP(cfg Config) (*HTTPClient, error) {
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN is required when SLACK_PROVIDER=slack")
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://slack.com/api"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ch := cfg.DefaultChannel
	if ch == "" {
		ch = "#incidents"
	}
	return &HTTPClient{token: token, defaultCh: ch, baseURL: base, client: &http.Client{Timeout: timeout}}, nil
}

func (c *HTTPClient) Provider() string { return integrations.ProviderSlack }

func (c *HTTPClient) PostMessage(ctx context.Context, channel, message string) (*integrations.SlackMessage, error) {
	if channel == "" {
		channel = c.defaultCh
	}
	if strings.TrimSpace(message) == "" {
		return nil, fmt.Errorf("message is required")
	}
	payload, _ := json.Marshal(map[string]string{
		"channel": channel,
		"text":    message,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat.postMessage", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if !parsed.OK {
		return nil, fmt.Errorf("slack api: %s", parsed.Error)
	}
	return &integrations.SlackMessage{Channel: firstNonEmpty(parsed.Channel, channel), MessageID: parsed.TS, OK: true}, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
