package v2

import (
	"encoding/json"
	"strings"
)

// NormalizeJSONObjectJSON 校验 match_labels 等字段：须为 JSON 对象（非数组）；空串归一为 "{}"。
func NormalizeJSONObjectJSON(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "{}", nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return "", err
	}
	return s, nil
}
