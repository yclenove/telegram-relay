package postgres

import (
	"fmt"
	"strings"
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

// buildAuditWhere 拼接审计列表 WHERE 子句与占位参数。
func buildAuditWhere(action, objectType string) (clause string, args []any) {
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
	return strings.Join(parts, " AND "), args
}
