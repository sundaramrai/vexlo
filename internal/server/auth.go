package server

import (
	"errors"
	"time"

	"github.com/sundaramrai/vexlo/internal/protocol"
	"github.com/sundaramrai/vexlo/internal/storage"
)

func (m *TunnelManager) validateRegistration(reg protocol.Register) error {
	if reg.LocalPort <= 0 || reg.LocalPort > 65535 {
		return errors.New("invalid local port")
	}
	if !reg.Quick {
		return errors.New("client must support quick tunnels")
	}
	if reg.SessionID != "" {
		return m.validateResume(reg)
	}
	return nil
}

func (m *TunnelManager) validateResume(reg protocol.Register) error {
	session, err := m.storage.GetSession(reg.SessionID)
	if err != nil {
		return errors.New("invalid session resume")
	}
	if !session.Hosted {
		return errors.New("invalid session mode")
	}
	if time.Since(session.StartedAt) >= m.cfg.HostedLifetime ||
		(session.EndedAt != nil && time.Since(*session.EndedAt) > 30*time.Second) {
		return errors.New("quick tunnel expired")
	}
	if reg.ResumeToken == "" || !subtleConstantTimeCompare(storage.HashHostedSecret(reg.ResumeToken), session.TunnelToken) {
		return errors.New("invalid resume token")
	}
	return nil
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
