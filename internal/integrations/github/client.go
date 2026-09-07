package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Revati-Firke/production-incident-investigator/internal/integrations"
)

// Client talks to GitHub or returns mock data.
type Client interface {
	Provider() string
	SearchCommits(ctx context.Context, service string, since time.Time) ([]integrations.Commit, error)
	InspectCode(ctx context.Context, path string) ([]integrations.CodeFinding, error)
	CreateIssue(ctx context.Context, title, body string) (*integrations.Issue, error)
	CreatePullRequest(ctx context.Context, title, branch, body string) (*integrations.PullRequest, error)
}

// MockClient returns deterministic GitHub-shaped data.
type MockClient struct{}

func NewMock() *MockClient { return &MockClient{} }

func (c *MockClient) Provider() string { return integrations.ProviderMock }

func (c *MockClient) SearchCommits(_ context.Context, _ string, _ time.Time) ([]integrations.Commit, error) {
	return []integrations.Commit{
		{SHA: "a1b2c3d", Message: "refactor: update repository connection handling", Author: "dev@example.com"},
	}, nil
}

func (c *MockClient) InspectCode(_ context.Context, path string) ([]integrations.CodeFinding, error) {
	if path == "" {
		path = "internal/repository/payment.go"
	}
	return []integrations.CodeFinding{
		{Path: path, Finding: "Connection acquired but not released in error path (line 142)"},
	}, nil
}

func (c *MockClient) CreateIssue(_ context.Context, title, _ string) (*integrations.Issue, error) {
	return &integrations.Issue{Number: 42, URL: "https://github.com/example/repo/issues/42", Title: title}, nil
}

func (c *MockClient) CreatePullRequest(_ context.Context, title, _, _ string) (*integrations.PullRequest, error) {
	return &integrations.PullRequest{Number: 17, URL: "https://github.com/example/repo/pull/17", Title: title}, nil
}

// HTTPClient uses GitHub REST API.
type HTTPClient struct {
	baseURL      string
	token        string
	owner        string
	repo         string
	client       *http.Client
	writeEnabled bool
}

// Config configures GitHub HTTP client.
type Config struct {
	Token        string
	Repository   string // owner/repo
	BaseURL      string
	Timeout      time.Duration
	WriteEnabled bool
}

// NewHTTP creates a GitHub HTTP client.
func NewHTTP(cfg Config) (*HTTPClient, error) {
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN is required when GITHUB_PROVIDER=github")
	}
	owner, repo, err := parseRepo(cfg.Repository)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &HTTPClient{
		baseURL:      base,
		token:        token,
		owner:        owner,
		repo:         repo,
		client:       &http.Client{Timeout: timeout},
		writeEnabled: cfg.WriteEnabled,
	}, nil
}

func (c *HTTPClient) Provider() string { return integrations.ProviderGitHub }

func (c *HTTPClient) SearchCommits(ctx context.Context, _ string, since time.Time) ([]integrations.Commit, error) {
	path := fmt.Sprintf("/repos/%s/%s/commits?per_page=10", c.owner, c.repo)
	if !since.IsZero() {
		path += "&since=" + since.UTC().Format(time.RFC3339)
	}
	var raw []struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
			Author  struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}
	out := make([]integrations.Commit, 0, len(raw))
	for _, item := range raw {
		author := item.Commit.Author.Email
		if author == "" {
			author = item.Commit.Author.Name
		}
		sha := item.SHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		out = append(out, integrations.Commit{
			SHA:     sha,
			Message: firstLine(item.Commit.Message),
			Author:  author,
			URL:     item.HTMLURL,
		})
	}
	return out, nil
}

func (c *HTTPClient) InspectCode(ctx context.Context, path string) ([]integrations.CodeFinding, error) {
	if path == "" {
		path = "README.md"
	}
	var raw struct {
		HTMLURL  string `json:"html_url"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		Message  string `json:"message"`
	}
	apiPath := fmt.Sprintf("/repos/%s/%s/contents/%s", c.owner, c.repo, strings.TrimPrefix(path, "/"))
	if err := c.get(ctx, apiPath, &raw); err != nil {
		return nil, err
	}
	finding := "File retrieved successfully"
	content := raw.Content
	if raw.Encoding == "base64" {
		if decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(raw.Content, "\n", "")); err == nil {
			content = string(decoded)
		}
	}
	lower := strings.ToLower(content)
	if strings.Contains(lower, "acquire") && !strings.Contains(lower, "release") {
		finding = "Possible connection acquire without matching release"
	} else if strings.Contains(lower, "todo") || strings.Contains(lower, "fixme") {
		finding = "Contains TODO/FIXME markers near recent change area"
	}
	return []integrations.CodeFinding{{
		Path:    path,
		Finding: finding,
		URL:     raw.HTMLURL,
	}}, nil
}

func (c *HTTPClient) CreateIssue(ctx context.Context, title, body string) (*integrations.Issue, error) {
	if !c.writeEnabled {
		return nil, fmt.Errorf("GitHub write disabled (set GITHUB_WRITE_ENABLED=true)")
	}
	payload, _ := json.Marshal(map[string]string{"title": title, "body": body})
	var raw struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		Title   string `json:"title"`
	}
	if err := c.post(ctx, fmt.Sprintf("/repos/%s/%s/issues", c.owner, c.repo), payload, &raw); err != nil {
		return nil, err
	}
	return &integrations.Issue{Number: raw.Number, URL: raw.HTMLURL, Title: raw.Title}, nil
}

func (c *HTTPClient) CreatePullRequest(ctx context.Context, title, branch, body string) (*integrations.PullRequest, error) {
	if !c.writeEnabled {
		return nil, fmt.Errorf("GitHub write disabled (set GITHUB_WRITE_ENABLED=true)")
	}
	if branch == "" {
		branch = "opspilot/fix"
	}
	payload, _ := json.Marshal(map[string]string{
		"title": title,
		"head":  branch,
		"base":  "main",
		"body":  body,
	})
	var raw struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		Title   string `json:"title"`
	}
	if err := c.post(ctx, fmt.Sprintf("/repos/%s/%s/pulls", c.owner, c.repo), payload, &raw); err != nil {
		return nil, err
	}
	return &integrations.PullRequest{Number: raw.Number, URL: raw.HTMLURL, Title: raw.Title}, nil
}

func (c *HTTPClient) get(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	c.auth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("github request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("github status %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	return json.Unmarshal(body, dest)
}

func (c *HTTPClient) post(ctx context.Context, path string, payload []byte, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	c.auth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("github request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("github status %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	return json.Unmarshal(body, dest)
}

func (c *HTTPClient) auth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

func parseRepo(repo string) (owner, name string, err error) {
	parts := strings.Split(strings.TrimSpace(repo), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("GITHUB_REPOSITORY must be owner/repo")
	}
	return parts[0], parts[1], nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
