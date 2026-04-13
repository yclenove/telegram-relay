package v2

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseListLimitOffset(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?limit=50&offset=10", nil)
	l, o := parseListLimitOffset(r)
	if l != 50 || o != 10 {
		t.Fatalf("got limit=%d offset=%d", l, o)
	}
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	l, o = parseListLimitOffset(r)
	if l != 20 || o != 0 {
		t.Fatalf("defaults got limit=%d offset=%d", l, o)
	}
	r = httptest.NewRequest(http.MethodGet, "/?limit=9999", nil)
	l, o = parseListLimitOffset(r)
	if l != 200 {
		t.Fatalf("cap limit got %d", l)
	}
	r = httptest.NewRequest(http.MethodGet, "/?limit=-5&offset=-1", nil)
	l, o = parseListLimitOffset(r)
	if l != 1 || o != 0 {
		t.Fatalf("negative clamp got limit=%d offset=%d", l, o)
	}
}

func TestParseOptionalRFC3339(t *testing.T) {
	tm, err := parseOptionalRFC3339("")
	if err != nil || tm != nil {
		t.Fatalf("empty: %v %v", tm, err)
	}
	tm, err = parseOptionalRFC3339("2026-04-01T12:00:00Z")
	if err != nil || tm == nil || !tm.Equal(time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("parse: %v err=%v", tm, err)
	}
	_, err = parseOptionalRFC3339("not-a-date")
	if err == nil {
		t.Fatal("expected error")
	}
}
