package relayhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/yclenove/telegram-relay/internal/model"
	"github.com/yclenove/telegram-relay/internal/relay"
	"github.com/yclenove/telegram-relay/internal/security"
	"github.com/yclenove/telegram-relay/internal/service"
)

type Handler struct {
	logger    *slog.Logger
	verifier  *security.CompositeVerifier
	relaySvc  *relay.Service
	notifySvc *service.NotifyService
	limiter   *rate.Limiter
	metrics   metrics
}

type metrics struct {
	total   uint64
	success uint64
	failed  uint64
}

func NewHandler(logger *slog.Logger, verifier *security.CompositeVerifier, relaySvc *relay.Service, notifySvc *service.NotifyService, limiter *rate.Limiter) *Handler {
	return &Handler{
		logger:   logger,
		verifier: verifier,
		relaySvc: relaySvc,
		notifySvc: notifySvc,
		limiter:  limiter,
	}
}

// Register 注册全部 HTTP 路由。
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/metrics", h.metricsHandler)
	mux.HandleFunc("/api/v1/notify", h.notify)
}

// healthz 用于健康检查，通常给负载均衡或监控系统探活。
func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// metricsHandler 提供最小可用指标，便于快速观察处理成功率。
func (h *Handler) metricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	out := fmt.Sprintf(
		"relay_requests_total %d\nrelay_requests_success %d\nrelay_requests_failed %d\n",
		atomic.LoadUint64(&h.metrics.total),
		atomic.LoadUint64(&h.metrics.success),
		atomic.LoadUint64(&h.metrics.failed),
	)
	_, _ = w.Write([]byte(out))
}

// notify 是核心入站处理流程：
// 1) 方法和限流校验
// 2) 读取请求体
// 3) 安全验证
// 4) JSON 反序列化和字段校验
// 5) 调用中转服务发送到 Telegram
func (h *Handler) notify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.limiter.Allow() {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	requestID := requestIDFromHeaders(r)

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		h.respondError(w, requestID, http.StatusBadRequest, "read body failed", err)
		return
	}
	atomic.AddUint64(&h.metrics.total, 1)

	nr, err := h.verifier.VerifyRequest(r, body)
	if err != nil {
		h.respondError(w, requestID, http.StatusUnauthorized, "security verification failed", err)
		return
	}
	ctx := context.WithValue(nr.Context(), "request_id", requestID)
	credID, _ := security.IngestCredentialIDFromContext(nr.Context())

	var req model.NotifyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.respondError(w, requestID, http.StatusBadRequest, "invalid json payload", err)
		return
	}
	if err := validateNotifyRequest(req, credID); err != nil {
		h.respondError(w, requestID, http.StatusBadRequest, "invalid notify request", err)
		return
	}

	if h.notifySvc != nil {
		if _, err := h.notifySvc.Ingest(ctx, req, body, credID); err != nil {
			h.respondError(w, requestID, http.StatusBadGateway, "event ingest failed", err)
			return
		}
	} else {
		sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := h.relaySvc.Send(sendCtx, req); err != nil {
			h.respondError(w, requestID, http.StatusBadGateway, "telegram relay failed", err)
			return
		}
	}

	atomic.AddUint64(&h.metrics.success, 1)
	h.logger.Info("relay message success", "request_id", requestID, "event_id", req.EventID, "source", req.Source)

	w.Header().Set("Content-Type", "application/json")
	resp := model.NotifyResponse{RequestID: requestID, Status: "queued"}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) respondError(w http.ResponseWriter, requestID string, code int, msg string, err error) {
	atomic.AddUint64(&h.metrics.failed, 1)
	h.logger.Error(msg, "request_id", requestID, "error", err, "status_code", code)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"request_id": requestID,
		"error":      msg,
	})
}

// validateNotifyRequest 校验中转请求的关键业务字段。
func validateNotifyRequest(req model.NotifyRequest, ingestCredentialID *int64) error {
	if strings.TrimSpace(req.Title) == "" {
		return errors.New("title is required")
	}
	if strings.TrimSpace(req.Message) == "" {
		return errors.New("message is required")
	}
	if strings.TrimSpace(req.Source) == "" && ingestCredentialID == nil {
		return errors.New("source is required")
	}
	if strings.TrimSpace(req.EventID) == "" {
		return errors.New("event_id is required")
	}
	return nil
}

// requestIDFromHeaders 优先复用上游 request id，便于链路追踪；
// 若不存在则生成本地 request id。
func requestIDFromHeaders(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Request-ID")); v != "" {
		return v
	}
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}
