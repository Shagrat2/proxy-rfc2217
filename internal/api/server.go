package api

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"time"

	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/config"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/device"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/session"
)

// Server is the HTTP API server
type Server struct {
	cfg      *config.Config
	handlers *Handlers
	server   *http.Server
}

// NewServer creates a new API server
func NewServer(cfg *config.Config, registry *device.Registry, sessions *session.Manager) *Server {
	handlers := NewHandlers(cfg, registry, sessions)

	mux := http.NewServeMux()

	// Health endpoints (no auth)
	mux.HandleFunc("/healthz", handlers.Healthz)
	mux.HandleFunc("/readyz", handlers.Readyz)

	// API endpoints (no auth for read, auth for write)
	mux.HandleFunc("/api/v1/devices", handlers.ListDevices)
	mux.HandleFunc("/api/v1/sessions", handlers.ListSessions)
	mux.HandleFunc("/api/v1/sessions/", handlers.TerminateSession) // requires auth
	mux.HandleFunc("/api/v1/stats", handlers.Stats)

	// Login endpoint
	mux.HandleFunc("/login", handlers.Login)
	mux.HandleFunc("/logout", handlers.Logout)

	// Web dashboard (no auth required, but shows more info when logged in)
	mux.HandleFunc("/", handlers.Dashboard)

	server := &http.Server{
		Addr:         ":" + cfg.APIPort,
		Handler:      logMiddleware(mux, cfg.DebugHTTP),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return &Server{
		cfg:      cfg,
		handlers: handlers,
		server:   server,
	}
}

// Start starts the API server
func (s *Server) Start(ctx context.Context) error {
	log.Printf("[api] server listening on %s", s.server.Addr)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(shutdownCtx)
	}()

	err := s.server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// logMiddleware logs HTTP requests when debug is enabled
func logMiddleware(next http.Handler, debug bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if debug {
			log.Printf("[api] %s %s %v", r.Method, r.URL.Path, time.Since(start))
		}
	})
}

// checkAuth checks if request has valid basic auth credentials
func checkAuth(r *http.Request, username, password string) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(user), []byte(username)) == 1 &&
		subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1
}
