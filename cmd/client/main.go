package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/sundaramrai/vexlo/internal/client"
)

func main() {
	cfg := client.DefaultConfig()
	flag.StringVar(&cfg.ServerAddr, "server", "127.0.0.1:9000", "binary server address")
	flag.StringVar(&cfg.RegisterToken, "register-token", "", "shared registration token required by the server")
	flag.BoolVar(&cfg.EnableTLS, "tls", false, "use TLS for the binary tunnel connection")
	flag.StringVar(&cfg.ServerName, "server-name", "", "TLS server name; defaults to the host in --server")
	flag.DurationVar(&cfg.RequestTimeout, "request-timeout", cfg.RequestTimeout, "local app request timeout")
	flag.Int64Var(&cfg.MaxResponseBodyBytes, "max-response-body-bytes", cfg.MaxResponseBodyBytes, "max local app response body forwarded through the tunnel")
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
