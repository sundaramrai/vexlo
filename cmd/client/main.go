package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/sundaramrai/vexlo/internal/client"
)

func main() {
	cfg := client.DefaultConfig()
	if len(os.Args) != 3 || os.Args[1] != "http" {
		log.Fatal("usage: vexlo http <local-port>")
	}
	port, err := strconv.Atoi(os.Args[2])
	if err != nil || port < 1 || port > 65535 {
		log.Fatal("local port must be between 1 and 65535")
	}
	cfg.LocalPort = port
	cfg.ServerAddr = "vexlo.duckdns.org:9000"
	cfg.EnableTLS = true
	cfg.ServerName = "vexlo.duckdns.org"
	run(cfg)
}

func run(cfg client.Config) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := client.Run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}
