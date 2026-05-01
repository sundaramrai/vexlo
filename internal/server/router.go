package server

import (
	"net/http"
	"slices"
	"strings"
)

func (t *Tunnel) ActiveRules() []RoutingRule {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return append([]RoutingRule(nil), t.rules...)
}

func (t *Tunnel) SetRules(rules []RoutingRule) {
	t.mu.Lock()
	defer t.mu.Unlock()
	sorted := append([]RoutingRule(nil), rules...)
	slices.SortFunc(sorted, func(a, b RoutingRule) int { return a.Priority - b.Priority })
	t.rules = sorted
}

func (t *Tunnel) ResolveTargetPort(r *http.Request) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, rule := range t.rules {
		if rule.MatchMethod != "" && !strings.EqualFold(rule.MatchMethod, r.Method) {
			continue
		}
		if rule.MatchPath != "" && !strings.Contains(r.URL.Path, rule.MatchPath) {
			continue
		}
		if rule.MatchHeaderKey != "" {
			if !strings.Contains(r.Header.Get(rule.MatchHeaderKey), rule.MatchHeaderValue) {
				continue
			}
		}
		return rule.TargetPort
	}
	return t.LocalPort
}
