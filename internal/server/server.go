package server

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sundaramrai/vexlo/internal/dashboard"
	"github.com/sundaramrai/vexlo/internal/storage"
)

type Server struct {
	cfg            Config
	manager        *TunnelManager
	db             *storage.DB
	hub            *dashboard.Hub
	httpSrv        *http.Server
	httpPlain      *http.Server
	tcpLn          net.Listener
	binarySlots    chan struct{}
	websocketSlots chan struct{}
	closeOnce      sync.Once
}

func New(cfg Config) (*Server, error) {
	if !cfg.Hosted || !cfg.EnableTLS || !cfg.EnableTunnelTLS || !strings.HasPrefix(cfg.HostURL, "https://") ||
		cfg.BaseDomain == "" || cfg.BaseDomain == "localhost" || cfg.HostedMaxTunnels <= 0 ||
		cfg.HostedMaxPerIP <= 0 || cfg.HostedMaxInFlight <= 0 || cfg.HostedLifetime <= 0 || cfg.HostedRetention <= 0 ||
		cfg.AdminUsername == "" || cfg.AdminPassword == "" {
		return nil, errors.New("hosted mode must be explicitly enabled and requires public HTTPS, tunnel TLS, a domain, positive limits, and operator credentials")
	}
	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := db.EndOrphanedHostedSessions(time.Now().UTC()); err != nil {
		_ = db.Close()
		return nil, err
	}
	hub := dashboard.NewHub()
	manager := NewTunnelManager(cfg, db)
	paused, err := db.HostedRegistrationsPaused()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	manager.registrationsPaused.Store(paused)
	srv := &Server{
		cfg:     cfg,
		db:      db,
		hub:     hub,
		manager: manager,
	}
	srv.binarySlots = make(chan struct{}, cfg.HostedMaxTunnels+100)
	srv.websocketSlots = make(chan struct{}, cfg.HostedMaxTunnels*2)
	return srv, nil
}

func (s *Server) Start(ctx context.Context) error {
	go s.hub.Run(ctx)
	go s.db.RunWriter(ctx)
	go s.runRetention(ctx)

	mux := s.routes()
	s.httpSrv = &http.Server{
		Addr:              s.cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       s.cfg.ReadTimeout,
		WriteTimeout:      s.cfg.WriteTimeout,
		IdleTimeout:       s.cfg.IdleTimeout,
	}
	s.httpSrv.MaxHeaderBytes = 16 * 1024

	tcpLn, err := net.Listen("tcp", s.cfg.TCPAddr)
	if err != nil {
		return err
	}
	if s.cfg.EnableTunnelTLS {
		tlsConfig, configErr := s.tunnelTLSConfig()
		if configErr != nil {
			_ = tcpLn.Close()
			return configErr
		}
		tcpLn = tls.NewListener(tcpLn, tlsConfig)
	}
	s.tcpLn = tcpLn
	go s.acceptBinary(ctx)

	go func() {
		<-ctx.Done()
		s.shutdown(context.Background())
	}()

	slog.Info("server listeners ready",
		"http_addr", s.cfg.HTTPAddr,
		"tcp_addr", s.cfg.TCPAddr,
		"tls_enabled", s.cfg.EnableTLS,
	)
	if s.cfg.EnableTLS {
		if err := os.MkdirAll(s.cfg.ACMECache, 0o755); err != nil {
			return err
		}
		slog.Info("https listener ready", "https_addr", s.cfg.HTTPSAddr)
		return s.serveTLS(ctx)
	}
	if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) runRetention(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	prune := func() {
		if s.cfg.RetentionPeriod > 0 {
			cutoff := time.Now().UTC().Add(-s.cfg.RetentionPeriod)
			if err := s.db.PruneBefore(cutoff); err != nil {
				slog.Warn("retention prune failed", "error", err, "cutoff", cutoff)
			}
		}
		s.manager.expireHostedTunnels(time.Now())
		cutoff := time.Now().UTC().Add(-s.cfg.HostedRetention)
		if err := s.db.PruneHostedBefore(cutoff); err != nil {
			slog.Warn("hosted retention prune failed", "error", err, "cutoff", cutoff)
		}
	}

	prune()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
}

func (s *Server) shutdown(ctx context.Context) {
	s.closeOnce.Do(func() {
		if s.httpSrv != nil {
			_ = s.httpSrv.Shutdown(ctx)
		}
		if s.httpPlain != nil {
			_ = s.httpPlain.Shutdown(ctx)
		}
		if s.tcpLn != nil {
			_ = s.tcpLn.Close()
		}
		_ = s.db.Close()
	})
}
