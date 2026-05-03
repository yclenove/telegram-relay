package v2

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/yclenove/telegram-relay/internal/repository/postgres"
	"github.com/yclenove/telegram-relay/internal/service"
)

// genIngestSecrets 生成 key_id、Bearer 明文、HMAC 明文及落库材料（bcrypt + Base64 enc）。
func genIngestSecrets(keyIDPrefix string) (keyID, plainToken, hmacPlain, tokenHash, hmacEnc string, err error) {
	if keyIDPrefix == "" {
		kb := make([]byte, 6)
		if _, err := rand.Read(kb); err != nil {
			return "", "", "", "", "", err
		}
		keyID = hex.EncodeToString(kb)
	} else {
		keyID = keyIDPrefix
	}
	sec := make([]byte, 24)
	if _, err := rand.Read(sec); err != nil {
		return "", "", "", "", "", err
	}
	secretPart := base64.RawURLEncoding.EncodeToString(sec)
	plainToken = keyID + "." + secretPart
	hm := make([]byte, 32)
	if _, err := rand.Read(hm); err != nil {
		return "", "", "", "", "", err
	}
	hmacPlain = hex.EncodeToString(hm)
	tokenHash, err = service.HashPassword(plainToken)
	if err != nil {
		return "", "", "", "", "", err
	}
	hmacEnc = postgres.EncryptSecret(hmacPlain)
	return keyID, plainToken, hmacPlain, tokenHash, hmacEnc, nil
}

func (h *Handler) ingestCredentials(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.store.ListIngestCredentials(r.Context())
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		writeJSON(w, jsonSlice(items))
	case http.MethodPost:
		var req struct {
			Name           string `json:"name"`
			Remark         string `json:"remark"`
			ExpiresInDays  *int   `json:"expires_in_days"` // 省略或 <=0：永不过期；1～3650：自创建时起算天数
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		var expiresAt *time.Time
		if req.ExpiresInDays != nil && *req.ExpiresInDays > 0 {
			if *req.ExpiresInDays > 3650 {
				http.Error(w, "expires_in_days too large (max 3650)", http.StatusBadRequest)
				return
			}
			t := time.Now().UTC().Add(time.Duration(*req.ExpiresInDays) * 24 * time.Hour)
			expiresAt = &t
		}
		var id int64
		var keyID, plainToken, hmacPlain, th, he string
		var createErr error
		for range 8 {
			var genErr error
			keyID, plainToken, hmacPlain, th, he, genErr = genIngestSecrets("")
			if genErr != nil {
				h.serverError(w, r, genErr)
				return
			}
			id, createErr = h.store.CreateIngestCredential(r.Context(), keyID, th, he, strings.TrimSpace(req.Name), strings.TrimSpace(req.Remark), expiresAt)
			if createErr == nil {
				break
			}
			var pe *pgconn.PgError
			if errors.As(createErr, &pe) && pe.Code == "23505" {
				id = 0
				continue
			}
			h.serverError(w, r, createErr)
			return
		}
		if id == 0 {
			http.Error(w, "could not allocate unique key_id", http.StatusConflict)
			return
		}
		h.writeAuditFromRequest(r, "ingest_credential.create", "ingest_credential", strconv.FormatInt(id, 10), map[string]any{"key_id": keyID, "name": req.Name})
		resp := map[string]any{
			"id":                id,
			"key_id":            keyID,
			"name":              strings.TrimSpace(req.Name),
			"plain_token":       plainToken,
			"plain_hmac_secret": hmacPlain,
			"bearer_preview":    "Authorization: Bearer " + plainToken,
		}
		if expiresAt != nil {
			resp["expires_at"] = expiresAt.UTC().Format(time.RFC3339)
		}
		writeJSON(w, resp)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) patchIngestCredential(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req struct {
		Name      *string `json:"name"`
		Remark    *string `json:"remark"`
		IsEnabled *bool   `json:"is_enabled"`
	}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	raw := make(map[string]json.RawMessage)
	if err := dec.Decode(&raw); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if v, ok := raw["name"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			http.Error(w, "invalid name", http.StatusBadRequest)
			return
		}
		req.Name = &s
	}
	if v, ok := raw["remark"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			http.Error(w, "invalid remark", http.StatusBadRequest)
			return
		}
		req.Remark = &s
	}
	if v, ok := raw["is_enabled"]; ok {
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			http.Error(w, "invalid is_enabled", http.StatusBadRequest)
			return
		}
		req.IsEnabled = &b
	}
	var expiresAtSet bool
	var expiresAt *time.Time
	if v, ok := raw["expires_in_days"]; ok {
		var n int64
		if err := json.Unmarshal(v, &n); err != nil {
			http.Error(w, "invalid expires_in_days", http.StatusBadRequest)
			return
		}
		expiresAtSet = true
		switch {
		case n < 0:
			expiresAt = nil
		case n == 0:
			expiresAtSet = false
		case n > 3650:
			http.Error(w, "expires_in_days too large (max 3650)", http.StatusBadRequest)
			return
		default:
			t := time.Now().UTC().Add(time.Duration(n) * 24 * time.Hour)
			expiresAt = &t
		}
	}
	out, err := h.store.UpdateIngestCredentialPatch(r.Context(), id, req.Name, req.Remark, req.IsEnabled, expiresAtSet, expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		h.serverError(w, r, err)
		return
	}
	h.writeAuditFromRequest(r, "ingest_credential.update", "ingest_credential", idStr, req)
	writeJSON(w, out)
}

func (h *Handler) rotateIngestCredential(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	row, err := h.store.GetIngestCredentialByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		h.serverError(w, r, err)
		return
	}
	keyID, plainToken, hmacPlain, th, he, err := genIngestSecrets(row.KeyID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if err := h.store.RotateIngestCredentialSecrets(r.Context(), id, th, he); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		h.serverError(w, r, err)
		return
	}
	h.writeAuditFromRequest(r, "ingest_credential.rotate", "ingest_credential", idStr, map[string]any{"key_id": keyID})
	resp := map[string]any{
		"id":                id,
		"key_id":            keyID,
		"name":              strings.TrimSpace(row.Name),
		"plain_token":       plainToken,
		"plain_hmac_secret": hmacPlain,
		"bearer_preview":    "Authorization: Bearer " + plainToken,
	}
	if row.ExpiresAt != nil {
		resp["expires_at"] = row.ExpiresAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, resp)
}
