package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
)

func writeJSON(w http.ResponseWriter, status int, resp APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func writeSuccess(w http.ResponseWriter, status int, data interface{}) {
	writeJSON(w, status, APIResponse{Success: true, Data: data, Error: nil})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, APIResponse{
		Success: false,
		Data:    nil,
		Error:   &APIError{Code: code, Message: message},
	})
}

func mapInvestigationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, invdomain.ErrNotFound):
		writeError(w, http.StatusNotFound, "INVESTIGATION_NOT_FOUND", "Investigation not found")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "INCIDENT_NOT_FOUND", "Incident not found")
	default:
		slog.Error("internal error", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred")
	}
}

// recoveryMiddleware recovers from panics and returns a 500 response.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered",
					"panic", rec,
					"stack", string(debug.Stack()),
					"path", r.URL.Path,
				)
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware adds baseline security headers.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs HTTP requests with structured fields.
func loggingMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)
			log.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
				"request_id", r.Header.Get("X-Request-Id"),
			)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
