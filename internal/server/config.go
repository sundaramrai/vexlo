package server

type Config struct {
	HTTPAddr   string
	HTTPSAddr  string
	TCPAddr    string
	SSHAddr    string
	BaseDomain string
	DBPath     string
	HostURL    string
	EnableTLS  bool
	ACMEEmail  string
	ACMECache  string
}

func DefaultConfig() Config {
	return Config{
		HTTPAddr:   ":8080",
		HTTPSAddr:  ":8443",
		TCPAddr:    ":9000",
		SSHAddr:    ":2222",
		BaseDomain: "localhost",
		DBPath:     "vexlo.db",
		HostURL:    "http://localhost:8080",
		ACMECache:  "acme-cache",
	}
}
