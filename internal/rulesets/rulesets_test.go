package rulesets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadAndCache(t *testing.T) {
	var requests int
	var etagSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("If-None-Match") == `"v1"` {
			etagSeen = true
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("SRS\x05test-payload"))
	}))
	defer srv.Close()

	m := New(t.TempDir())
	m.Items = []Item{{Tag: "test", Title: "Test", URL: srv.URL, File: "test.srs"}}

	if err := m.Update(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}
	path, ok := m.Path("test")
	if !ok {
		t.Fatal("rule-set not installed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "SRS\x05test-payload" {
		t.Fatalf("data = %q", data)
	}

	if err := m.Update(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}
	if !etagSeen {
		t.Fatal("ETag was not sent on the second update")
	}
	if requests != 2 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestInvalidRuleSetIsRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not an srs file"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	m := New(dir)
	m.Items = []Item{{Tag: "bad", Title: "Bad", URL: srv.URL, File: "bad.srs"}}
	if err := m.Update(context.Background(), "bad"); err == nil {
		t.Fatal("expected an error")
	}
	if _, ok := m.Path("bad"); ok {
		t.Fatal("invalid file was installed")
	}
	if _, err := os.Stat(filepath.Join(dir, "bad.srs")); !os.IsNotExist(err) {
		t.Fatalf("invalid file left behind: %v", err)
	}
}

func TestStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("SRS\x05data"))
	}))
	defer srv.Close()

	m := New(t.TempDir())
	m.Items = []Item{{Tag: "one", Title: "One", URL: srv.URL, File: "one.srs"}}
	status := m.Status()
	if len(status) != 1 || status[0].Installed {
		t.Fatalf("status = %+v", status)
	}
	if err := m.Update(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	status = m.Status()
	if !status[0].Installed || status[0].Size == 0 {
		t.Fatalf("status = %+v", status)
	}
}
