package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"vexlo/internal/client"
)

func main() {
	cfg := client.DefaultConfig()
	flag.StringVar(&cfg.ServerAddr, "server", "127.0.0.1:9000", "binary server address")
	flag.StringVar(&cfg.Scheme, "scheme", "http", "server scheme used in output URLs")
	flag.StringVar(&cfg.Mode, "mode", "tcp", "connection mode: tcp|ssh|ws")
	flag.StringVar(&cfg.HostURL, "host-url", "http://localhost:8080", "dashboard base URL")
	flag.StringVar(&cfg.SSHAddr, "ssh-addr", "127.0.0.1:2222", "SSH server address")
	flag.StringVar(&cfg.WSURL, "ws-url", "ws://127.0.0.1:8080/ws/tunnel", "WebSocket tunnel URL")
	flag.StringVar(&cfg.RegisterToken, "register-token", "", "shared registration token required by the server for tcp/ws tunnel registration")
	flag.Parse()

	if flag.NArg() > 0 {
		port, err := strconv.Atoi(flag.Arg(0))
		if err != nil {
			log.Fatalf("invalid port: %v", err)
		}
		cfg.LocalPort = port
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := client.Run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}
