package rulesets

import (
	"bytes"
	"compress/zlib"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func testSRS() []byte {
	var body bytes.Buffer
	writer := zlib.NewWriter(&body)
	_, _ = writer.Write([]byte{0}) // one empty rule
	_ = writer.Close()
	return append([]byte("SRS\x05"), body.Bytes()...)
}

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
		_, _ = w.Write([]byte(testSRS()))
	}))
	defer srv.Close()

	m := New(t.TempDir())
	m.SetItems([]Item{{Tag: "test", Title: "Test", URL: srv.URL, File: "test.srs"}})

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
	if !bytes.Equal(data, testSRS()) {
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
	m.SetItems([]Item{{Tag: "bad", Title: "Bad", URL: srv.URL, File: "bad.srs"}})
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

func TestTruncatedRuleSetRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("SRS\x05"))
	}))
	defer srv.Close()

	m := New(t.TempDir())
	m.SetItems([]Item{{Tag: "bad", Title: "Bad", URL: srv.URL, File: "bad.srs"}})
	if err := m.Update(context.Background(), "bad"); err == nil {
		t.Fatal("truncated rule-set was accepted")
	}
	if _, ok := m.Path("bad"); ok {
		t.Fatal("truncated rule-set is reported as installed")
	}
}

func TestNotModifiedWithoutFileIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	m := New(t.TempDir())
	m.SetItems([]Item{{Tag: "one", Title: "One", URL: srv.URL, File: "one.srs"}})
	if err := m.Update(context.Background(), "one"); err == nil {
		t.Fatal("304 without a local file was reported as success")
	}
}

func TestOversizeRuleSetRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.srs")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(MaxSize + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	_ = file.Close()

	m := New(dir)
	m.SetItems([]Item{{Tag: "big", Title: "Big", File: "big.srs"}})
	if _, ok := m.Path("big"); ok {
		t.Fatal("oversized rule-set is reported as installed")
	}
}

func TestStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testSRS()))
	}))
	defer srv.Close()

	m := New(t.TempDir())
	m.SetItems([]Item{{Tag: "one", Title: "One", URL: srv.URL, File: "one.srs"}})
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

func TestRedirectMustStayHTTPS(t *testing.T) {
	m := New(t.TempDir())
	httpReq, _ := http.NewRequest(http.MethodGet, "http://example.com/rule.srs", nil)
	if err := m.HTTP.CheckRedirect(httpReq, nil); err == nil {
		t.Fatal("HTTP redirect was allowed")
	}
	httpsReq, _ := http.NewRequest(http.MethodGet, "https://example.com/rule.srs", nil)
	if err := m.HTTP.CheckRedirect(httpsReq, nil); err != nil {
		t.Fatalf("HTTPS redirect rejected: %v", err)
	}
}
