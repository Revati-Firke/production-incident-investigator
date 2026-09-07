package integrations

import "time"

// Provider names used in config and status responses.
const (
	ProviderMock    = "mock"
	ProviderLoki    = "loki"
	ProviderProm    = "prometheus"
	ProviderGrafana = "grafana"
	ProviderGitHub  = "github"
	ProviderSlack   = "slack"
)

// Status describes which external systems are wired.
type Status struct {
	Loki       EndpointStatus `json:"loki"`
	Prometheus EndpointStatus `json:"prometheus"`
	Grafana    EndpointStatus `json:"grafana"`
	GitHub     EndpointStatus `json:"github"`
	Slack      EndpointStatus `json:"slack"`
}

// EndpointStatus is one integration's configured mode.
type EndpointStatus struct {
	Provider   string `json:"provider"`
	Configured bool   `json:"configured"`
	Details    string `json:"details,omitempty"`
}

// LogEntry is a normalized log line from Loki or mocks.
type LogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// LogSearchRequest is input for log search.
type LogSearchRequest struct {
	Service     string
	Environment string
	Query       string
	Limit       int
	TimeRange   time.Duration
}

// MetricSample is a normalized metric point.
type MetricSample struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit,omitempty"`
	Max   float64 `json:"max,omitempty"`
}

// MetricsQueryRequest is input for metrics queries.
type MetricsQueryRequest struct {
	Service     string
	Environment string
	Metric      string
}

// ServiceHealth is normalized health output.
type ServiceHealth struct {
	Service      string              `json:"service"`
	Environment  string              `json:"environment"`
	Status       string              `json:"status"`
	Dependencies []map[string]string `json:"dependencies"`
	Source       string              `json:"source,omitempty"`
}

// Commit is a normalized GitHub commit.
type Commit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	Author  string `json:"author"`
	URL     string `json:"url,omitempty"`
}

// CodeFinding is a normalized code inspection result.
type CodeFinding struct {
	Path    string `json:"path"`
	Finding string `json:"finding"`
	URL     string `json:"url,omitempty"`
}

// Issue is a created GitHub issue.
type Issue struct {
	Number int    `json:"issue_number"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}

// PullRequest is a created GitHub PR.
type PullRequest struct {
	Number int    `json:"pr_number"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}

// SlackMessage is a sent notification result.
type SlackMessage struct {
	Channel   string `json:"channel"`
	MessageID string `json:"message_id"`
	OK        bool   `json:"ok"`
}
