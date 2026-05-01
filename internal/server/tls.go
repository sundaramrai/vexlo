package server

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net/http"
	"strings"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

func (s *Server) serveTLS(ctx context.Context) error {
	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(s.cfg.ACMECache),
		Email:      s.cfg.ACMEEmail,
		HostPolicy: s.autocertHostPolicy(),
	}

	redirectMux := http.NewServeMux()
	redirectMux.Handle("/", manager.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + stripPort(r.Host)
		if s.cfg.HTTPSAddr != ":443" && s.cfg.HTTPSAddr != "" {
			target += s.cfg.HTTPSAddr
		}
		target += r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
	})))

	s.httpPlain = &http.Server{
		Addr:    s.cfg.HTTPAddr,
		Handler: redirectMux,
	}
	go func() {
		if err := s.httpPlain.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("plain HTTP redirect server stopped: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = s.httpPlain.Shutdown(context.Background())
	}()

	s.httpSrv.Addr = s.cfg.HTTPSAddr
	s.httpSrv.TLSConfig = &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: manager.GetCertificate,
		NextProtos:     []string{acme.ALPNProto, "h2", "http/1.1"},
	}
	err := s.httpSrv.ListenAndServeTLS("", "")
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) autocertHostPolicy() autocert.HostPolicy {
	return func(_ context.Context, host string) error {
		host = stripPort(strings.ToLower(host))
		base := strings.ToLower(strings.TrimSpace(s.cfg.BaseDomain))
		if base == "" {
			return errors.New("empty base domain")
		}
		if host == base || host == "www."+base {
			return nil
		}
		if strings.HasSuffix(host, "."+base) {
			return nil
		}
		return errors.New("acme host rejected")
	}
}

func stripPort(host string) string {
	if i := strings.Index(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}
