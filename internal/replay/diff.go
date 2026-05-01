package replay

import (
	"encoding/json"
	"reflect"
)

type DiffResult struct {
	Added   map[string]any        `json:"added,omitempty"`
	Removed map[string]any        `json:"removed,omitempty"`
	Changed map[string]DiffChange `json:"changed,omitempty"`
	Same    map[string]any        `json:"same,omitempty"`
	Text    []TextChange          `json:"text,omitempty"`
	Mode    string                `json:"mode"`
}

type DiffChange struct {
	From any `json:"from"`
	To   any `json:"to"`
}

type TextChange struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func CompareBodies(original, replayed string) (*DiffResult, error) {
	var left any
	var right any
	if json.Unmarshal([]byte(original), &left) == nil && json.Unmarshal([]byte(replayed), &right) == nil {
		return compareJSON(left, right), nil
	}
	return compareText(original, replayed), nil
}

func compareJSON(original, replayed any) *DiffResult {
	res := &DiffResult{
		Added:   map[string]any{},
		Removed: map[string]any{},
		Changed: map[string]DiffChange{},
		Same:    map[string]any{},
		Mode:    "json",
	}
	omap, ok1 := original.(map[string]any)
	rmap, ok2 := replayed.(map[string]any)
	if !ok1 || !ok2 {
		compareLeafValues(res, original, replayed)
		return res
	}
	for key, value := range omap {
		compareMapEntry(res, key, value, rmap)
	}
	for key, value := range rmap {
		if _, ok := omap[key]; !ok {
			res.Added[key] = value
		}
	}
	return res
}

func compareMapEntry(res *DiffResult, key string, value any, rmap map[string]any) {
	rv, ok := rmap[key]
	if !ok {
		res.Removed[key] = value
		return
	}
	if reflect.DeepEqual(value, rv) {
		res.Same[key] = value
		return
	}
	childLeft, leftMap := value.(map[string]any)
	childRight, rightMap := rv.(map[string]any)
	if leftMap && rightMap {
		res.Changed[key] = DiffChange{From: compareJSON(childLeft, childRight), To: compareJSON(childRight, childLeft)}
		return
	}
	res.Changed[key] = DiffChange{From: value, To: rv}
}

func compareLeafValues(res *DiffResult, original, replayed any) {
	if !reflect.DeepEqual(original, replayed) {
		res.Changed["value"] = DiffChange{From: original, To: replayed}
		return
	}
	res.Same["value"] = original
}

func compareText(original, replayed string) *DiffResult {
	if original == replayed {
		return &DiffResult{Mode: "text", Text: []TextChange{{Type: "same", Value: original}}}
	}
	return &DiffResult{
		Mode: "text",
		Text: []TextChange{
			{Type: "removed", Value: original},
			{Type: "added", Value: replayed},
		},
	}
}
