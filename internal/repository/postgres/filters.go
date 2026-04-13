package postgres

import (
	"fmt"
	"strings"
	"time"
)

// buildEventWhere 拼接事件列表 WHERE 子句与占位参数（精确匹配非空筛选项）。
func buildEventWhere(source, level, status string) (clause string, args []any) {
	args = []any{}
	parts := []string{"TRUE"}
	if t := strings.TrimSpace(source); t != "" {
		args = append(args, t)
		parts = append(parts, fmt.Sprintf("source = $%d", len(args)))
	}
	if t := strings.TrimSpace(level); t != "" {
		args = append(args, t)
		parts = append(parts, fmt.Sprintf("level = $%d", len(args)))
	}
	if t := strings.TrimSpace(status); t != "" {
		args = append(args, t)
		parts = append(parts, fmt.Sprintf("status = $%d", len(args)))
	}
	return strings.Join(parts, " AND "), args
}

// buildAuditWhere 拼接审计列表 WHERE 与参数；支持时间范围（含边界，与 PostgreSQL timestamptz 比较）。
func buildAuditWhere(
	action, objectType, objectID string,
	actorUserID *int64,
	createdAfter, createdBefore *time.Time,
) (clause string, args []any) {
	args = []any{}
	parts := []string{"TRUE"}
	if t := strings.TrimSpace(action); t != "" {
		args = append(args, t)
		parts = append(parts, fmt.Sprintf("action = $%d", len(args)))
	}
	if t := strings.TrimSpace(objectType); t != "" {
		args = append(args, t)
		parts = append(parts, fmt.Sprintf("object_type = $%d", len(args)))
	}
	if t := strings.TrimSpace(objectID); t != "" {
		args = append(args, t)
		parts = append(parts, fmt.Sprintf("object_id = $%d", len(args)))
	}
	if actorUserID != nil {
		args = append(args, *actorUserID)
		parts = append(parts, fmt.Sprintf("actor_user_id = $%d", len(args)))
	}
	if createdAfter != nil {
		args = append(args, *createdAfter)
		parts = append(parts, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if createdBefore != nil {
		args = append(args, *createdBefore)
		parts = append(parts, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	return strings.Join(parts, " AND "), args
}

// buildDispatchWhere 拼接 dispatch_jobs 列表 WHERE（按 status 精确匹配，空则不过滤）。
func buildDispatchWhere(status string) (clause string, args []any) {
	args = []any{}
	parts := []string{"TRUE"}
	if t := strings.TrimSpace(status); t != "" {
		args = append(args, t)
		parts = append(parts, fmt.Sprintf("status = $%d", len(args)))
	}
	return strings.Join(parts, " AND "), args
}
