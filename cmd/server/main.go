package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"vexlo/internal/server"
)

func main() {
	cfg := server.DefaultConfig()
	flag.StringVar(&cfg.HTTPAddr, "http-addr", ":8080", "dashboard/public HTTP listen address")
	flag.StringVar(&cfg.HTTPSAddr, "https-addr", ":8443", "public HTTPS listen address")
	flag.StringVar(&cfg.TCPAddr, "tcp-addr", ":9000", "binary tunnel listen address")
	flag.StringVar(&cfg.SSHAddr, "ssh-addr", ":2222", "SSH tunnel listen address")
	flag.StringVar(&cfg.BaseDomain, "base-domain", "localhost", "base domain used for tunnel host mapping")
	flag.StringVar(&cfg.DBPath, "db", "vexlo.db", "SQLite database path")
	flag.StringVar(&cfg.HostURL, "host-url", "http://localhost:8080", "dashboard base URL returned to clients")
	flag.BoolVar(&cfg.EnableTLS, "tls", false, "enable HTTPS with Let's Encrypt autocert")
	flag.StringVar(&cfg.ACMEEmail, "acme-email", "", "email used for Let's Encrypt registration")
	flag.StringVar(&cfg.ACMECache, "acme-cache", "acme-cache", "directory used to cache Let's Encrypt certificates")
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
