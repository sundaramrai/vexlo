package server

import (
	"net/http"
	"slices"
	"strings"
)

func (t *Tunnel) ActiveRules() []RoutingRule {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cp := append([]RoutingRule(nil), t.rules...)
	slices.SortFunc(cp, func(a, b RoutingRule) int { return a.Priority - b.Priority })
	return cp
}

func (t *Tunnel) SetRules(rules []RoutingRule) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rules = append([]RoutingRule(nil), rules...)
}

func (t *Tunnel) ResolveTargetPort(r *http.Request) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	rules := append([]RoutingRule(nil), t.rules...)
	slices.SortFunc(rules, func(a, b RoutingRule) int { return a.Priority - b.Priority })
	for _, rule := range rules {
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
