package server

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"

	"vexlo/internal/protocol"
)

func (m *TunnelManager) validateRegistration(reg protocol.Register) error {
	if reg.LocalPort <= 0 {
		return errors.New("invalid local port")
	}
	if reg.SessionID != "" {
		session, err := m.storage.GetSession(reg.SessionID)
		if err != nil {
			return errors.New("invalid session resume")
		}
		if reg.ResumeToken == "" || reg.ResumeToken != session.TunnelToken {
			return errors.New("invalid resume token")
		}
		return nil
	}
	if reg.ConnectionType == "ssh" {
		if !m.isAuthorizedSSHPublicKey(reg.PublicKey) {
			return errors.New("unauthorized ssh key")
		}
		return nil
	}
	if m.cfg.RegistrationToken == "" {
		return errors.New("registration token not configured")
	}
	if subtleConstantTimeCompare(reg.ClientToken, m.cfg.RegistrationToken) {
		return nil
	}
	return errors.New("invalid registration token")
}

func subtleConstantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func loadAuthorizedKeys(path string) (map[string]struct{}, error) {
	keys := map[string]struct{}{}
	if strings.TrimSpace(path) == "" {
		return keys, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	for len(raw) > 0 {
		pub, _, _, rest, err := ssh.ParseAuthorizedKey(raw)
		if err != nil {
			return nil, err
		}
		keys[string(ssh.MarshalAuthorizedKey(pub))] = struct{}{}
		raw = rest
	}
	return keys, nil
}

func (m *TunnelManager) isAuthorizedSSHPublicKey(key string) bool {
	if len(m.authorizedSSHKeys) == 0 {
		return false
	}
	normalized := string(bytes.TrimSpace([]byte(key)))
	_, ok := m.authorizedSSHKeys[normalized+"\n"]
	if ok {
		return true
	}
	_, ok = m.authorizedSSHKeys[normalized]
	return ok
}

func loadOrCreateSSHSigner(path string) (ssh.Signer, error) {
	if raw, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(raw)
		if block == nil {
			return nil, errors.New("invalid ssh host key pem")
		}
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return ssh.NewSignerFromKey(key)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(key)
}
