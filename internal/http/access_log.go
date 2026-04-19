package relayhttp

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// statusRecorder 包装 ResponseWriter 以捕获最终 HTTP 状态码，供访问日志使用。
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

// accessLogRequestID 优先复用客户端 X-Request-ID，便于与上游网关对齐；否则生成本地 ID。
func accessLogRequestID(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Request-ID")); v != "" {
		return v
	}
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}

// AccessLog 为每个请求打一条 INFO 日志（方法、路径、状态、耗时、来源地址、请求 ID），便于本地与生产排障。
// 说明：日志输出到 stdout（与现有 slog JSON 一致），不会在「审计日志」表里重复记录。
// 同时回写 X-Request-ID 响应头，便于调用方做端到端关联。
func AccessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := accessLogRequestID(r)
		w.Header().Set("X-Request-ID", reqID)

		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)

		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		logger.Info("http_access",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", rec.code,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", host,
			"request_id", reqID,
		)
	})
}
