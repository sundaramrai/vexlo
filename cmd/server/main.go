package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vexlo/internal/server"
)

func main() {
	cfg := server.DefaultConfig()
	flag.StringVar(&cfg.HTTPAddr, "http-addr", ":8080", "dashboard/public HTTP listen address")
	flag.StringVar(&cfg.HTTPSAddr, "https-addr", ":8443", "public HTTPS listen address")
	flag.StringVar(&cfg.TCPAddr, "tcp-addr", ":9000", "binary tunnel listen address")
	flag.StringVar(&cfg.BaseDomain, "base-domain", "localhost", "base domain used for tunnel host mapping")
	flag.StringVar(&cfg.DBPath, "db", "vexlo.db", "SQLite database path")
	flag.StringVar(&cfg.HostURL, "host-url", "http://localhost:8080", "dashboard base URL returned to clients")
	flag.BoolVar(&cfg.EnableTLS, "tls", false, "enable HTTPS with Let's Encrypt autocert")
	flag.StringVar(&cfg.ACMEEmail, "acme-email", "", "email used for Let's Encrypt registration")
	flag.StringVar(&cfg.ACMECache, "acme-cache", "acme-cache", "directory used to cache Let's Encrypt certificates")
	flag.IntVar(&cfg.CaptureBodyLimit, "capture-body-limit", 256*1024, "max request/response body bytes to retain for dashboard storage; 0 disables the limit")
	flag.Int64Var(&cfg.MaxRequestBodyBytes, "max-request-body-bytes", 2*1024*1024, "max inbound tunneled request body size in bytes")
	flag.Int64Var(&cfg.MaxAPIBodyBytes, "max-api-body-bytes", 512*1024, "max API request body size in bytes")
	flag.DurationVar(&cfg.ReadTimeout, "read-timeout", 15*time.Second, "HTTP server read timeout")
	flag.DurationVar(&cfg.WriteTimeout, "write-timeout", 60*time.Second, "HTTP server write timeout")
	flag.DurationVar(&cfg.IdleTimeout, "idle-timeout", 60*time.Second, "HTTP server idle timeout")
	flag.StringVar(&cfg.RegistrationToken, "registration-token", "", "shared token required for tunnel registration")
	flag.DurationVar(&cfg.RetentionPeriod, "retention-period", 7*24*time.Hour, "how long ended sessions and captured traffic are retained; 0 disables pruning")
	flag.StringVar(&cfg.AdminUsername, "admin-user", "", "optional HTTP basic auth username for dashboard and management APIs")
	flag.StringVar(&cfg.AdminPassword, "admin-pass", "", "optional HTTP basic auth password for dashboard and management APIs")
	flag.Parse()

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Start(ctx); err != nil {
		log.Fatal(err)
	}
}
