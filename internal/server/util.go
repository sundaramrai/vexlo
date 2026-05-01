package server

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
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

func captureResponseBody(headers http.Header, raw []byte, limit int) string {
	decoded := decodeResponseBody(headers, raw)
	return captureBody(decoded, limit)
}

func decodeResponseBody(headers http.Header, raw []byte) []byte {
	encoding := strings.ToLower(strings.TrimSpace(headers.Get("Content-Encoding")))
	if !strings.Contains(encoding, "gzip") {
		return raw
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return raw
	}
	defer func() { _ = reader.Close() }()

	decoded, err := io.ReadAll(reader)
	if err != nil {
		return raw
	}
	return decoded
}
