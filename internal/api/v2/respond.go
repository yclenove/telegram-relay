package v2

import (
	"net/http"
)

// serverError 记录内部错误并返回固定文案，避免将数据库/栈信息泄露给客户端。
func (h *Handler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		h.logger.Error("internal server error", "path", r.URL.Path, "error", err)
	}
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

// badGateway 用于入队/下游失败等场景，同样不向客户端暴露内部原因。
func (h *Handler) badGateway(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		h.logger.Error("bad gateway", "path", r.URL.Path, "error", err)
	}
	http.Error(w, "service temporarily unavailable", http.StatusBadGateway)
}

// badRequestLogged 将不可信来源的错误记入日志，对外统一为简短文案。
func (h *Handler) badRequestLogged(w http.ResponseWriter, r *http.Request, public string, internal error) {
	if internal != nil {
		h.logger.Warn("bad request", "path", r.URL.Path, "public", public, "error", internal)
	}
	http.Error(w, public, http.StatusBadRequest)
}
