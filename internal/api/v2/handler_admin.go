package v2

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/yclenove/telegram-relay/internal/service"
)

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := h.store.ListRoles(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	writeJSON(w, jsonSlice(list))
}

// listRolePermissions 返回指定角色的权限码列表（只读），便于管理台展示 RBAC 而不改库。
func (h *Handler) listRolePermissions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	rid, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || rid <= 0 {
		http.Error(w, "invalid role id", http.StatusBadRequest)
		return
	}
	codes, err := h.store.ListPermissionCodesForRole(r.Context(), rid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		h.serverError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"role_id": rid, "permissions": jsonSlice(codes)})
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := h.store.ListUserSummaries(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	writeJSON(w, jsonSlice(list))
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username  string  `json:"username"`
		Password  string  `json:"password"`
		IsEnabled *bool   `json:"is_enabled"`
		RoleIDs   []int64 `json:"role_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}
	enabled := true
	if req.IsEnabled != nil {
		enabled = *req.IsEnabled
	}
	if _, err := h.store.GetUserByUsername(r.Context(), req.Username); err == nil {
		http.Error(w, "username already exists", http.StatusConflict)
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.serverError(w, r, err)
		return
	}
	if err := h.store.ValidateRoleIDsExist(r.Context(), req.RoleIDs); err != nil {
		h.badRequestLogged(w, r, "invalid role ids", err)
		return
	}
	hash, err := service.HashPassword(req.Password)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	out, err := h.store.CreateUserWithRoles(r.Context(), req.Username, hash, enabled, req.RoleIDs)
	if err != nil {
		h.badRequestLogged(w, r, "invalid request", err)
		return
	}
	h.writeAuditFromRequest(r, "user.create", "user", strconv.FormatInt(out.ID, 10), req)
	writeJSON(w, out)
}

func (h *Handler) patchUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	uid, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || uid <= 0 {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	if _, err := h.store.GetUserByID(r.Context(), uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		h.serverError(w, r, err)
		return
	}
	var req struct {
		IsEnabled *bool    `json:"is_enabled"`
		Password  string   `json:"password"`
		RoleIDs   *[]int64 `json:"role_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	var pwdHash *string
	if strings.TrimSpace(req.Password) != "" {
		ph, herr := service.HashPassword(req.Password)
		if herr != nil {
			h.serverError(w, r, herr)
			return
		}
		pwdHash = &ph
	}
	if req.RoleIDs != nil {
		if err := h.store.ValidateRoleIDsExist(r.Context(), *req.RoleIDs); err != nil {
			h.badRequestLogged(w, r, "invalid role ids", err)
			return
		}
	}
	out, err := h.store.UpdateUserPatch(r.Context(), uid, req.IsEnabled, pwdHash, req.RoleIDs)
	if err != nil {
		if strings.Contains(err.Error(), "最后一个超级管理员") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.badRequestLogged(w, r, "invalid request", err)
		return
	}
	h.writeAuditFromRequest(r, "user.update", "user", strconv.FormatInt(uid, 10), req)
	writeJSON(w, out)
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	uid, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || uid <= 0 {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteUser(r.Context(), uid); err != nil {
		if strings.Contains(err.Error(), "最后一个超级管理员") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.serverError(w, r, err)
		return
	}
	h.writeAuditFromRequest(r, "user.delete", "user", idStr, map[string]any{"id": uid})
	w.WriteHeader(http.StatusNoContent)
}