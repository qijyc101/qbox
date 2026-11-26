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
