package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Server wraps the HTTP server.
type Server struct {
	server *http.Server
	log    *slog.Logger
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// NewServer creates a new HTTP server.
func NewServer(cfg ServerConfig, handler http.Handler, log *slog.Logger) *Server {
	return &Server{
		server: &http.Server{
			Addr:         ":" + cfg.Port,
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
		log: log,
	}
}

// Start begins serving HTTP requests.
func (s *Server) Start() error {
	s.log.Info("starting http server", "addr", s.server.Addr)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info("shutting down http server")
	return s.server.Shutdown(ctx)
}
