package v2

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"telegram-notification/internal/model"
	"telegram-notification/internal/repository/postgres"
	"telegram-notification/internal/service"
)

type contextKey string

const (
	contextUserID contextKey = "uid"
)

// Handler 提供 v2 管理 API。
type Handler struct {
	logger    *slog.Logger
	store     *postgres.Store
	authSvc   *service.AuthService
	notifySvc *service.NotifyService
}

func NewHandler(logger *slog.Logger, store *postgres.Store, authSvc *service.AuthService, notifySvc *service.NotifyService) *Handler {
	return &Handler{
		logger:    logger,
		store:     store,
		authSvc:   authSvc,
		notifySvc: notifySvc,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/auth/login", h.login)
	mux.Handle("/api/v2/bots", h.withAuth("bot.manage", http.HandlerFunc(h.bots)))
	mux.Handle("/api/v2/destinations", h.withAuth("bot.manage", http.HandlerFunc(h.destinations)))
	mux.Handle("/api/v2/rules", h.withAuth("rule.manage", http.HandlerFunc(h.rules)))
	mux.Handle("/api/v2/events", h.withAuth("event.read", http.HandlerFunc(h.events)))
	mux.Handle("/api/v2/dashboard", h.withAuth("event.read", http.HandlerFunc(h.dashboard)))
	mux.Handle("/api/v2/notify", http.HandlerFunc(h.notifyV2))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	token, perms, _, err := h.authSvc.Login(r.Context(), strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{"access_token": token, "permissions": perms})
}

func (h *Handler) bots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := h.store.ListBots(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for i := range list {
			list[i].BotTokenEnc = ""
		}
		writeJSON(w, list)
	case http.MethodPost:
		var req struct {
			Name      string `json:"name"`
			BotToken  string `json:"bot_token"`
			IsDefault bool   `json:"is_default"`
			Remark    string `json:"remark"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		created, err := h.store.CreateBot(r.Context(), req.Name, req.BotToken, req.Remark, req.IsDefault)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		created.BotTokenEnc = ""
		h.writeAuditFromRequest(r, "bot.create", "bot", strconv.FormatInt(created.ID, 10), req)
		writeJSON(w, created)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) destinations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		BotID     int64  `json:"bot_id"`
		Name      string `json:"name"`
		ChatID    string `json:"chat_id"`
		ParseMode string `json:"parse_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	out, err := h.store.CreateDestination(r.Context(), req.BotID, req.Name, req.ChatID, req.ParseMode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.writeAuditFromRequest(r, "destination.create", "destination", strconv.FormatInt(out.ID, 10), req)
	writeJSON(w, out)
}

func (h *Handler) rules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rules, err := h.store.ListRules(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, rules)
	case http.MethodPost:
		var req struct {
			Name          string `json:"name"`
			Priority      int    `json:"priority"`
			MatchSource   string `json:"match_source"`
			MatchLevel    string `json:"match_level"`
			DestinationID int64  `json:"destination_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		item, err := h.store.CreateRule(r.Context(), req.Name, req.Priority, req.MatchSource, req.MatchLevel, req.DestinationID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.writeAuditFromRequest(r, "rule.create", "rule", strconv.FormatInt(item.ID, 10), req)
		writeJSON(w, item)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	events, err := h.store.ListEvents(r.Context(), 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, events)
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stats, err := h.store.DashboardStats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (h *Handler) notifyV2(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	var req model.NotifyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Source) == "" || strings.TrimSpace(req.Message) == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}
	id, err := h.notifySvc.Ingest(r.Context(), req, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"event_db_id": id, "status": "queued"})
}

func (h *Handler) withAuth(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		uid, perms, err := h.authSvc.ParseToken(strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		if !perms[permission] && !perms["system.manage"] {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		ctx := r.Context()
		ctx = contextWithUID(ctx, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) writeAuditFromRequest(r *http.Request, action, objectType, objectID string, detail any) {
	uid, ok := uidFromContext(r.Context())
	if !ok {
		return
	}
	detailJSON := service.BuildAuditDetail(detail)
	if err := h.store.WriteAudit(r.Context(), &uid, action, objectType, objectID, detailJSON); err != nil {
		h.logger.Error("write audit failed", "error", err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func contextWithUID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, contextUserID, uid)
}

func uidFromContext(ctx context.Context) (int64, bool) {
	v := ctx.Value(contextUserID)
	uid, ok := v.(int64)
	return uid, ok
}
