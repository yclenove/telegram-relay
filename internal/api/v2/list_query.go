package v2

import (
	"net/http"
	"strconv"
)

// parseListLimitOffset 解析管理端列表分页参数：默认 limit=20，最大 200，offset 非负。
func parseListLimitOffset(r *http.Request) (limit, offset int) {
	q := r.URL.Query()
	limit = atoiDefault(q.Get("limit"), 20)
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}
	offset = atoiDefault(q.Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
