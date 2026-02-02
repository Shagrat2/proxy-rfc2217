package connection

import (
	"context"
	"log"
	"net"

	"github.com/pires/go-proxyproto"

	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/config"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/device"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/session"
)

// Server listens for all connections (devices and clients)
type Server struct {
	cfg      *config.Config
	handler  *Handler
	listener net.Listener
}

// NewServer creates a new connection server
func NewServer(cfg *config.Config, registry *device.Registry, sessions *session.Manager) *Server {
	return &Server{
		cfg:     cfg,
		handler: NewHandler(cfg, registry, sessions),
	}
}

// Start starts the connection server
func (s *Server) Start(ctx context.Context) error {
	addr := ":" + s.cfg.Port
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	// Wrap with PROXY Protocol support if enabled
	if s.cfg.ProxyProtocol {
		listener = &proxyproto.Listener{Listener: listener}
		log.Printf("[server] PROXY Protocol enabled")
	}

	s.listener = listener

	log.Printf("[server] listening on %s", addr)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				log.Printf("[server] accept error: %v", err)
				continue
			}
		}

		go s.handler.Handle(ctx, conn)
	}
}

// Addr returns the server address
func (s *Server) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}
