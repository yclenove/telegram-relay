package v2

import (
	"net/http"
)

// listDispatchJobs 分页列出异步发送任务，可选按 status 精确筛选（与事件只读权限一致，便于排障）。
func (h *Handler) listDispatchJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, offset := parseListLimitOffset(r)
	status := r.URL.Query().Get("status")
	items, total, err := h.store.ListDispatchJobs(r.Context(), status, limit, offset)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"items": jsonSlice(items), "total": total})
}
