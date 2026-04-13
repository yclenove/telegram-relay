package v2

import "testing"

func TestNormalizeJSONObjectJSON(t *testing.T) {
	s, err := NormalizeJSONObjectJSON("")
	if err != nil || s != "{}" {
		t.Fatalf("empty: %q %v", s, err)
	}
	s, err = NormalizeJSONObjectJSON(`  {"a":1}  `)
	if err != nil || s != `{"a":1}` {
		t.Fatalf("object: %q %v", s, err)
	}
	_, err = NormalizeJSONObjectJSON(`[]`)
	if err == nil {
		t.Fatal("array should fail")
	}
}
