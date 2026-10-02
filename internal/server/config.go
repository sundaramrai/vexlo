package server

import "time"

type Config struct {
	HTTPAddr            string
	HTTPSAddr           string
	TCPAddr             string
	BaseDomain          string
	DBPath              string
	HostURL             string
	EnableTLS           bool
	EnableTunnelTLS     bool
	ACMEEmail           string
	ACMECache           string
	TLSCertFile         string
	TLSKeyFile          string
	TLSExtraCertFile    string
	TLSExtraKeyFile     string
	CaptureBodyLimit    int
	MaxRequestBodyBytes int64
	MaxAPIBodyBytes     int64
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	RetentionPeriod     time.Duration
	AdminUsername       string
	AdminPassword       string
	Hosted              bool
	HostedMaxTunnels    int
	HostedMaxPerIP      int
	HostedMaxInFlight   int
	HostedLifetime      time.Duration
	HostedRetention     time.Duration
}

func DefaultConfig() Config {
	return Config{
		HTTPAddr:            ":8080",
		HTTPSAddr:           ":8443",
		TCPAddr:             ":9000",
		BaseDomain:          "localhost",
		DBPath:              "vexlo.db",
		HostURL:             "http://localhost:8080",
		ACMECache:           "acme-cache",
		CaptureBodyLimit:    256 * 1024,
		MaxRequestBodyBytes: 2 * 1024 * 1024,
		MaxAPIBodyBytes:     512 * 1024,
		ReadTimeout:         15 * time.Second,
		WriteTimeout:        60 * time.Second,
		IdleTimeout:         60 * time.Second,
		RetentionPeriod:     7 * 24 * time.Hour,
		HostedMaxTunnels:    100,
		HostedMaxPerIP:      5,
		HostedMaxInFlight:   100,
		HostedLifetime:      8 * time.Hour,
		HostedRetention:     time.Hour,
	}
}
