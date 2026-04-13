package v2

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
