package v2

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/yclenove/telegram-relay/internal/repository/postgres"
)

// patchBot 部分更新机器人（名称、备注、启用、默认、可选轮换 Token）。
func (h *Handler) patchBot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid bot id", http.StatusBadRequest)
		return
	}
	var req struct {
		Name       *string `json:"name"`
		Remark     *string `json:"remark"`
		IsEnabled  *bool   `json:"is_enabled"`
		IsDefault  *bool   `json:"is_default"`
		BotToken   *string `json:"bot_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	out, err := h.store.UpdateBotPatch(r.Context(), id, req.Name, req.Remark, req.IsEnabled, req.IsDefault, req.BotToken)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			http.Error(w, "duplicate name", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out.BotTokenEnc = ""
	h.writeAuditFromRequest(r, "bot.update", "bot", idStr, req)
	writeJSON(w, out)
}

// deleteBot 删除机器人（级联删除其目标与关联规则，由数据库外键保证）。
func (h *Handler) deleteBot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid bot id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteBot(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeAuditFromRequest(r, "bot.delete", "bot", idStr, map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

// patchRule 部分更新路由规则（名称、优先级、匹配条件、目标、启用）。
func (h *Handler) patchRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid rule id", http.StatusBadRequest)
		return
	}
	var req struct {
		Name           *string `json:"name"`
		Priority       *int    `json:"priority"`
		MatchSource    *string `json:"match_source"`
		MatchLevel     *string `json:"match_level"`
		MatchLabels    *string `json:"match_labels"`
		DestinationID  *int64  `json:"destination_id"`
		IsEnabled      *bool   `json:"is_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	var matchLabelsPatch *string
	if req.MatchLabels != nil {
		norm, err := NormalizeJSONObjectJSON(*req.MatchLabels)
		if err != nil {
			http.Error(w, "invalid match_labels: must be a JSON object", http.StatusBadRequest)
			return
		}
		matchLabelsPatch = &norm
	}
	out, err := h.store.UpdateRulePatch(r.Context(), id, req.Name, req.Priority, req.MatchSource, req.MatchLevel, matchLabelsPatch, req.DestinationID, req.IsEnabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, postgres.ErrDestinationNotFound) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			http.Error(w, "duplicate name", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.writeAuditFromRequest(r, "rule.update", "rule", idStr, req)
	writeJSON(w, out)
}

// deleteRule 删除单条路由规则。
func (h *Handler) deleteRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid rule id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteRule(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeAuditFromRequest(r, "rule.delete", "rule", idStr, map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}
