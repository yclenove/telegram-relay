package v2

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/time/rate"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/yclenove/telegram-relay/internal/model"
	"github.com/yclenove/telegram-relay/internal/repository/postgres"
	"github.com/yclenove/telegram-relay/internal/security"
	"github.com/yclenove/telegram-relay/internal/service"
)

type contextKey string

const (
	contextUserID contextKey = "uid"
)

// notifyIngester 由异步入队服务实现；测试可注入桩，避免依赖真实数据库。
type notifyIngester interface {
	Ingest(ctx context.Context, req model.NotifyRequest, rawBody []byte, ingestCredentialID *int64) (int64, error)
}

// Handler 提供 v2 管理 API。
type Handler struct {
	logger    *slog.Logger
	store     *postgres.Store
	authSvc   *service.AuthService
	notifySvc notifyIngester
	verifier  *security.CompositeVerifier
	limiter   *rate.Limiter
	hints     IntegrationHints
}

// NewHandler 组装 v2 处理器；verifier 与 limiter 用于 /api/v2/notify，与 v1 入站安全模型对齐。
// hints 用于接入文档与 PUBLIC_BASE_URL 提示；可传零值。
func NewHandler(logger *slog.Logger, store *postgres.Store, authSvc *service.AuthService, notifySvc notifyIngester, verifier *security.CompositeVerifier, limiter *rate.Limiter, hints IntegrationHints) *Handler {
	return &Handler{
		logger:    logger,
		store:     store,
		authSvc:   authSvc,
		notifySvc: notifySvc,
		verifier:  verifier,
		limiter:   limiter,
		hints:     hints,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/auth/login", h.login)
	mux.HandleFunc("/api/v2/auth/refresh", h.refresh)
	mux.Handle("/api/v2/bots", h.withAuth("bot.manage", http.HandlerFunc(h.bots)))
	mux.Handle("PATCH /api/v2/bots/{id}", h.withAuth("bot.manage", http.HandlerFunc(h.patchBot)))
	mux.Handle("DELETE /api/v2/bots/{id}", h.withAuth("bot.manage", http.HandlerFunc(h.deleteBot)))
	mux.Handle("/api/v2/destinations", h.withAuth("bot.manage", http.HandlerFunc(h.destinations)))
	mux.Handle("PATCH /api/v2/destinations/{id}", h.withAuth("bot.manage", http.HandlerFunc(h.patchDestination)))
	mux.Handle("DELETE /api/v2/destinations/{id}", h.withAuth("bot.manage", http.HandlerFunc(h.deleteDestination)))
	mux.Handle("/api/v2/rules", h.withAuth("rule.manage", http.HandlerFunc(h.rules)))
	mux.Handle("PATCH /api/v2/rules/{id}", h.withAuth("rule.manage", http.HandlerFunc(h.patchRule)))
	mux.Handle("DELETE /api/v2/rules/{id}", h.withAuth("rule.manage", http.HandlerFunc(h.deleteRule)))
	mux.Handle("/api/v2/events", h.withAuth("event.read", http.HandlerFunc(h.events)))
	mux.Handle("GET /api/v2/events/{id}", h.withAuth("event.read", http.HandlerFunc(h.getEvent)))
	mux.Handle("GET /api/v2/dispatch-jobs", h.withAuth("event.read", http.HandlerFunc(h.listDispatchJobs)))
	mux.Handle("/api/v2/audits", h.withAuth("audit.read", http.HandlerFunc(h.audits)))
	mux.Handle("/api/v2/dashboard", h.withAuth("event.read", http.HandlerFunc(h.dashboard)))
	// 与 /api/v1/notify 相同：Bearer/HMAC（按 security.level）+ 全局限流，避免公开入队被滥用。
	mux.Handle("/api/v2/notify", http.HandlerFunc(h.notifyV2Secure))
	// 管理端测试入队：走 JWT + bot.manage，避免浏览器持有 AUTH_TOKEN 直调公开 notify。
	mux.Handle("/api/v2/notify-test", h.withAuth("bot.manage", http.HandlerFunc(h.notifyTest)))
	// 用户与角色管理（Go 1.22+ 方法路由，与旧式路径匹配并存）
	mux.Handle("GET /api/v2/roles", h.withAuth("user.manage", http.HandlerFunc(h.listRoles)))
	mux.Handle("GET /api/v2/roles/{id}/permissions", h.withAuth("user.manage", http.HandlerFunc(h.listRolePermissions)))
	mux.Handle("GET /api/v2/users", h.withAuth("user.manage", http.HandlerFunc(h.listUsers)))
	mux.Handle("POST /api/v2/users", h.withAuth("user.manage", http.HandlerFunc(h.createUser)))
	mux.Handle("PATCH /api/v2/users/{id}", h.withAuth("user.manage", http.HandlerFunc(h.patchUser)))
	mux.Handle("DELETE /api/v2/users/{id}", h.withAuth("user.manage", http.HandlerFunc(h.deleteUser)))
	mux.Handle("/api/v2/ingest-credentials", h.withAuth("ingest_credential.manage", http.HandlerFunc(h.ingestCredentials)))
	mux.Handle("PATCH /api/v2/ingest-credentials/{id}", h.withAuth("ingest_credential.manage", http.HandlerFunc(h.patchIngestCredential)))
	mux.Handle("POST /api/v2/ingest-credentials/{id}/rotate", h.withAuth("ingest_credential.manage", http.HandlerFunc(h.rotateIngestCredential)))
	mux.Handle("GET /api/v2/rule-presets", h.withAuthAny([]string{"rule.manage", "bot.manage"}, http.HandlerFunc(h.rulePresets)))
	mux.Handle("GET /api/v2/integration-hints", h.withAuth("ingest_credential.manage", http.HandlerFunc(h.integrationHints)))
	mux.Handle("POST /api/v2/ingest-credentials/integration-doc", h.withAuth("ingest_credential.manage", http.HandlerFunc(h.renderIntegrationDoc)))
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
	access, refresh, perms, _, err := h.authSvc.Login(r.Context(), strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{
		"access_token":  access,
		"refresh_token": refresh,
		"permissions":   jsonSlice(perms),
	})
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	access, newRefresh, perms, err := h.authSvc.Refresh(r.Context(), strings.TrimSpace(req.RefreshToken))
	if err != nil {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{
		"access_token":  access,
		"refresh_token": newRefresh,
		"permissions":   jsonSlice(perms),
	})
}

func (h *Handler) bots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := h.store.ListBots(r.Context())
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		for i := range list {
			list[i].BotTokenEnc = ""
		}
		writeJSON(w, jsonSlice(list))
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
			h.badRequestLogged(w, r, "invalid request", err)
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
	switch r.Method {
	case http.MethodGet:
		list, err := h.store.ListDestinations(r.Context())
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		writeJSON(w, jsonSlice(list))
	case http.MethodPost:
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
			h.badRequestLogged(w, r, "invalid request", err)
			return
		}
		h.writeAuditFromRequest(r, "destination.create", "destination", strconv.FormatInt(out.ID, 10), req)
		writeJSON(w, out)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) rules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rules, err := h.store.ListRules(r.Context())
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		writeJSON(w, jsonSlice(rules))
	case http.MethodPost:
		var req struct {
			Name          string `json:"name"`
			Priority      int    `json:"priority"`
			MatchSource   string `json:"match_source"`
			MatchLevel    string `json:"match_level"`
			MatchLabels   string `json:"match_labels"`
			DestinationID int64  `json:"destination_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		ml, err := NormalizeJSONObjectJSON(req.MatchLabels)
		if err != nil {
			http.Error(w, "invalid match_labels: must be a JSON object", http.StatusBadRequest)
			return
		}
		item, err := h.store.CreateRule(r.Context(), req.Name, req.Priority, req.MatchSource, req.MatchLevel, ml, req.DestinationID)
		if err != nil {
			var pe *pgconn.PgError
			if errors.As(err, &pe) {
				switch pe.Code {
				case "23505": // routing_rules.name UNIQUE
					http.Error(w, "rule name already exists", http.StatusConflict)
					return
				case "23503": // destination_id FK
					http.Error(w, "destination not found", http.StatusBadRequest)
					return
				}
			}
			h.badRequestLogged(w, r, "invalid request", err)
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
	q := r.URL.Query()
	limit, offset := parseListLimitOffset(r)
	items, total, err := h.store.ListEvents(r.Context(), q.Get("source"), q.Get("level"), q.Get("status"), limit, offset)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"items": jsonSlice(items), "total": total})
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stats, err := h.store.DashboardStats(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	writeJSON(w, stats)
}

func (h *Handler) audits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	limit, offset := parseListLimitOffset(r)
	var actorPtr *int64
	if s := strings.TrimSpace(q.Get("actor_user_id")); s != "" {
		aid, err := strconv.ParseInt(s, 10, 64)
		if err != nil || aid <= 0 {
			http.Error(w, "invalid actor_user_id", http.StatusBadRequest)
			return
		}
		actorPtr = &aid
	}
	createdAfter, err := parseOptionalRFC3339(q.Get("created_after"))
	if err != nil {
		http.Error(w, "invalid created_after (use RFC3339)", http.StatusBadRequest)
		return
	}
	createdBefore, err := parseOptionalRFC3339(q.Get("created_before"))
	if err != nil {
		http.Error(w, "invalid created_before (use RFC3339)", http.StatusBadRequest)
		return
	}
	items, total, err := h.store.ListAuditLogs(r.Context(), q.Get("action"), q.Get("object_type"), q.Get("object_id"), actorPtr, createdAfter, createdBefore, limit, offset)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"items": jsonSlice(items), "total": total})
}

// notifyV2Secure 对入队接口执行与 v1 相同的限流与安全校验，再进入 JSON 校验与异步入队。
func (h *Handler) notifyV2Secure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.limiter != nil && !h.limiter.Allow() {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	if h.verifier != nil {
		nr, err := h.verifier.VerifyRequest(r, body)
		if err != nil {
			h.logger.Warn("notify v2 security verification failed", "error", err)
			http.Error(w, "security verification failed", http.StatusUnauthorized)
			return
		}
		r = nr
	}
	h.notifyV2Body(w, r, body)
}

func (h *Handler) notifyV2Body(w http.ResponseWriter, r *http.Request, body []byte) {
	var req model.NotifyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Message) == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}
	credID, _ := security.IngestCredentialIDFromContext(r.Context())
	if strings.TrimSpace(req.Source) == "" && credID == nil {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}
	id, err := h.notifySvc.Ingest(r.Context(), req, body, credID)
	if err != nil {
		if errors.Is(err, service.ErrNoDestination) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.badGateway(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"event_db_id": id, "status": "queued"})
}

func (h *Handler) notifyTest(w http.ResponseWriter, r *http.Request) {
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
	id, err := h.notifySvc.Ingest(r.Context(), req, body, nil)
	if err != nil {
		if errors.Is(err, service.ErrNoDestination) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.badGateway(w, r, err)
		return
	}
	h.writeAuditFromRequest(r, "notify.test", "event", strconv.FormatInt(id, 10), map[string]any{"source": req.Source, "title": req.Title})
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

// withAuthAny 满足任一权限即可（用于规则模板：建规则的人或管机器人的人都能看预设）。
func (h *Handler) withAuthAny(permissions []string, next http.Handler) http.Handler {
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
		if perms["system.manage"] {
			ctx := contextWithUID(r.Context(), uid)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		ok := false
		for _, p := range permissions {
			if perms[p] {
				ok = true
				break
			}
		}
		if !ok {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		ctx := contextWithUID(r.Context(), uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) writeAuditFromRequest(r *http.Request, action, objectType, objectID string, detail any) {
	if h.store == nil {
		return
	}
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

// jsonSlice 将 nil slice 规范为「空切片」，使 JSON 为 [] 而非 null，避免管理端表格/下拉读 length 崩溃。
func jsonSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func contextWithUID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, contextUserID, uid)
}

func uidFromContext(ctx context.Context) (int64, bool) {
	v := ctx.Value(contextUserID)
	uid, ok := v.(int64)
	return uid, ok
}
