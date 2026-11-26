package jsonutil

import (
	"encoding/json"
	"testing"
)

func TestSetObjectTag(t *testing.T) {
	tag, err := SetObjectTag(json.RawMessage(`{"type":"vless","server":"example.com"}`), "node")
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(tag, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["tag"] != "node" || obj["type"] != "vless" {
		t.Fatalf("result = %s", tag)
	}
}

func TestSetObjectTagRejectsNonObjects(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `"text"`, `0`, ``} {
		if _, err := SetObjectTag(json.RawMessage(raw), "tag"); err == nil {
			t.Errorf("%q was accepted", raw)
		}
	}
}
