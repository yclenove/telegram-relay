package v2

import (
	_ "embed"
	"encoding/json"
	"net/http"
)

//go:embed rule_presets.json
var rulePresetsJSON []byte

// rulePresets 返回内置路由场景模板（只读 JSON，无需数据库表）。
func (h *Handler) rulePresets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var items []map[string]any
	if err := json.Unmarshal(rulePresetsJSON, &items); err != nil {
		h.serverError(w, r, err)
		return
	}
	writeJSON(w, jsonSlice(items))
}
