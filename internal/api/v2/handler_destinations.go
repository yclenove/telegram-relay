package v2

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/yclenove/telegram-relay/internal/repository/postgres"
)

// patchDestination 部分更新发送目标（机器人、名称、Chat、Topic、格式、启用）。
func (h *Handler) patchDestination(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid destination id", http.StatusBadRequest)
		return
	}
	var req struct {
		BotID      *int64  `json:"bot_id"`
		Name       *string `json:"name"`
		ChatID     *string `json:"chat_id"`
		TopicID    *string `json:"topic_id"`
		ParseMode  *string `json:"parse_mode"`
		IsEnabled  *bool   `json:"is_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	out, err := h.store.UpdateDestinationPatch(r.Context(), id, req.BotID, req.Name, req.ChatID, req.TopicID, req.ParseMode, req.IsEnabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, postgres.ErrBotNotFound) {
			http.Error(w, "bot not found", http.StatusBadRequest)
			return
		}
		h.badRequestLogged(w, r, "invalid request", err)
		return
	}
	h.writeAuditFromRequest(r, "destination.update", "destination", idStr, req)
	writeJSON(w, out)
}

// deleteDestination 删除发送目标（关联路由规则由数据库级联删除）。
func (h *Handler) deleteDestination(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid destination id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteDestination(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		h.serverError(w, r, err)
		return
	}
	h.writeAuditFromRequest(r, "destination.delete", "destination", idStr, map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}
