package server

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	"vexlo/internal/model"
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

var sensitiveHeaderNames = []string{
	"authorization",
	"proxy-authorization",
	"cookie",
	"set-cookie",
	"x-api-key",
	"x-auth-token",
}

func sanitizeCapturedRequest(req model.CapturedRequest) model.CapturedRequest {
	req.Headers = sanitizeHeaderJSON(req.Headers)
	req.ResponseHeaders = sanitizeHeaderJSON(req.ResponseHeaders)
	req.DecodedHeaders = headerJSONToMap(req.Headers)
	if req.Replay != nil {
		replayCopy := sanitizeCapturedReplay(*req.Replay)
		req.Replay = &replayCopy
	}
	return req
}

func sanitizeCapturedReplay(replay model.CapturedReplay) model.CapturedReplay {
	replay.MutatedHeaders = sanitizeHeaderJSON(replay.MutatedHeaders)
	replay.ResponseHeader = sanitizeHeaderJSON(replay.ResponseHeader)
	return replay
}

func sanitizeHeaderJSON(raw string) string {
	if raw == "" {
		return raw
	}
	headers := map[string][]string{}
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return raw
	}
	for key := range headers {
		if isSensitiveHeader(key) {
			headers[key] = []string{"[redacted]"}
		}
	}
	buf, err := json.Marshal(headers)
	if err != nil {
		return raw
	}
	return string(buf)
}

func isSensitiveHeader(name string) bool {
	return slices.Contains(sensitiveHeaderNames, strings.ToLower(strings.TrimSpace(name)))
}
