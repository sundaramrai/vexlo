package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

func randomID(size int) string {
	buf := make([]byte, size)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func headerJSONToMap(raw string) map[string]string {
	if raw == "" {
		return map[string]string{}
	}
	src := map[string][]string{}
	if err := json.Unmarshal([]byte(raw), &src); err != nil {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = strings.Join(v, ", ")
	}
	return dst
}

func flattenHeaders(src http.Header) map[string]string {
	dst := make(map[string]string, len(src))
	for key, values := range src {
		dst[key] = strings.Join(values, ", ")
	}
	return dst
}

func captureBody(raw []byte, limit int) string {
	if limit <= 0 || len(raw) <= limit {
		return string(raw)
	}
	return string(raw[:limit]) + "\n...[truncated]"
}
