package server

import (
	"errors"

	"github.com/sundaramrai/vexlo/internal/protocol"
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
