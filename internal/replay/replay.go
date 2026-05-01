package replay

import "encoding/json"

func MustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}
