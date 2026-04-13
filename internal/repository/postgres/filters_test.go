package postgres

import "testing"

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
	clause, args := buildAuditWhere("bot.update", "")
	if want := "TRUE AND action = $1"; clause != want {
		t.Fatalf("clause got %q want %q", clause, want)
	}
	if len(args) != 1 || args[0] != "bot.update" {
		t.Fatalf("args %v", args)
	}
}
