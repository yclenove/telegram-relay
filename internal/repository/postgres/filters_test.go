package postgres

import (
	"testing"
	"time"
)

func TestBuildEventWhere(t *testing.T) {
	clause, args := buildEventWhere("", "", "")
	if clause != "TRUE" || len(args) != 0 {
		t.Fatalf("empty filters: clause=%q args=%v", clause, args)
	}
	clause, args = buildEventWhere("  svc  ", "warn", "")
	if want := "TRUE AND source = $1 AND level = $2"; clause != want {
		t.Fatalf("clause got %q want %q", clause, want)
	}
	if len(args) != 2 || args[0] != "svc" || args[1] != "warn" {
		t.Fatalf("args %v", args)
	}
}

func TestBuildAuditWhere(t *testing.T) {
	clause, args := buildAuditWhere("bot.update", "", "", nil, nil, nil)
	if want := "TRUE AND action = $1"; clause != want {
		t.Fatalf("clause got %q want %q", clause, want)
	}
	if len(args) != 1 || args[0] != "bot.update" {
		t.Fatalf("args %v", args)
	}
	aid := int64(7)
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	clause, args = buildAuditWhere("", "", "42", &aid, &t1, &t2)
	if want := "TRUE AND object_id = $1 AND actor_user_id = $2 AND created_at >= $3 AND created_at <= $4"; clause != want {
		t.Fatalf("clause got %q want %q", clause, want)
	}
	if len(args) != 4 || args[0] != "42" || args[1] != int64(7) || args[2] != t1 || args[3] != t2 {
		t.Fatalf("args %v", args)
	}
}

func TestBuildDispatchWhere(t *testing.T) {
	clause, args := buildDispatchWhere("")
	if clause != "TRUE" || len(args) != 0 {
		t.Fatalf("empty status: %q %v", clause, args)
	}
	clause, args = buildDispatchWhere("  pending  ")
	if want := "TRUE AND status = $1"; clause != want {
		t.Fatalf("got %q", clause)
	}
	if len(args) != 1 || args[0] != "pending" {
		t.Fatalf("args %v", args)
	}
}
