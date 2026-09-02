package http

// APIResponse is the standard API envelope.
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   *APIError   `json:"error"`
}

// APIError represents a structured API error.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PaginatedData wraps list responses with pagination metadata.
type PaginatedData struct {
	Items  interface{} `json:"items"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}
