package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"vexlo/internal/dashboard"
	"vexlo/internal/storage"
)

type Server struct {
	cfg       Config
	manager   *TunnelManager
	db        *storage.DB
	hub       *dashboard.Hub
	httpSrv   *http.Server
	httpPlain *http.Server
	tcpLn     net.Listener
	closeOnce sync.Once
}

func New(cfg Config) (*Server, error) {
	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	hub := dashboard.NewHub()
	manager := NewTunnelManager(cfg, db, hub)
	return &Server{
		cfg:     cfg,
		db:      db,
		hub:     hub,
		manager: manager,
	}, nil
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

	tcpLn, err := net.Listen("tcp", s.cfg.TCPAddr)
	if err != nil {
		return err
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
	if s.cfg.RetentionPeriod <= 0 {
		return
	}
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	prune := func() {
		cutoff := time.Now().UTC().Add(-s.cfg.RetentionPeriod)
		if err := s.db.PruneBefore(cutoff); err != nil {
			slog.Warn("retention prune failed", "error", err, "cutoff", cutoff)
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
