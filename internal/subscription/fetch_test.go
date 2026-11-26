package subscription

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetch(t *testing.T) {
	const body = "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls#one"
	var userAgent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := NewClient()
	data, err := client.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Fatalf("data = %q", data)
	}
	if !strings.Contains(userAgent, "qbox") {
		t.Fatalf("user agent = %q", userAgent)
	}
}

func TestFetchErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient()
	if _, err := client.Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("expected a status error")
	}
	if _, err := client.Fetch(context.Background(), "ftp://example.com/sub"); err == nil {
		t.Fatal("expected a scheme error")
	}
	if _, err := client.Fetch(context.Background(), ""); err == nil {
		t.Fatal("expected an empty URL error")
	}
}

func TestRedirectDowngradeBlocked(t *testing.T) {
	client := NewClient()
	makeReq := func(raw string) *http.Request {
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		return req
	}
	cases := []struct {
		name    string
		req     *http.Request
		via     []*http.Request
		wantErr bool
	}{
		{"https to http", makeReq("http://example.com/x"), []*http.Request{makeReq("https://example.com/sub")}, true},
		{"http to https to http", makeReq("http://example.com/x"), []*http.Request{makeReq("http://example.com/sub"), makeReq("https://cdn.example.com/sub")}, true},
		{"https to https", makeReq("https://cdn.example.com/x"), []*http.Request{makeReq("https://example.com/sub")}, false},
		{"ftp redirect", makeReq("ftp://example.com/x"), []*http.Request{makeReq("https://example.com/sub")}, true},
	}
	for _, tt := range cases {
		err := client.HTTP.CheckRedirect(tt.req, tt.via)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestFetchErrorDoesNotLeakToken(t *testing.T) {
	client := NewClient()
	_, err := client.Fetch(context.Background(), "http://127.0.0.1:1/sub?token=SECRET")
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("token leaked in error: %v", err)
	}
}

func TestRedactURL(t *testing.T) {
	got := redactURL("https://user:pass@example.com/sub/TOKEN?token=SECRET#frag")
	if strings.Contains(got, "TOKEN") || strings.Contains(got, "SECRET") || strings.Contains(got, "user") {
		t.Fatalf("redactURL = %q", got)
	}
	if got != "https://example.com/…" {
		t.Fatalf("redactURL = %q", got)
	}
}
