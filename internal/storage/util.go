package storage

import (
	"encoding/json"
	"strings"
)

func HeaderJSONToMap(raw string) map[string]string {
	if raw == "" {
		return map[string]string{}
	}
	src := map[string][]string{}
	if err := json.Unmarshal([]byte(raw), &src); err != nil {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for key, values := range src {
		dst[key] = strings.Join(values, ", ")
	}
	return dst
}
