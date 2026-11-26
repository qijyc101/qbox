// Package subscription fetches and parses proxy subscriptions.
//
// Supported inputs are base64-wrapped or plain URI lists (vless, vmess,
// trojan, ss, hysteria2, tuic, socks, http) and sing-box JSON documents
// with an "outbounds" array. Clash YAML is intentionally not supported in
// this version.
package subscription

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// MaxBodySize bounds a subscription download.
	MaxBodySize = 10 << 20
	userAgent   = "qbox/0.1 (sing-box 1.14.4)"
)

// Client downloads subscriptions directly, without the tunnel.
type Client struct {
	HTTP *http.Client
}

// NewClient returns a client with sensible timeouts. An HTTPS subscription
// may not be redirected to plain HTTP.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("перенаправление на %q запрещено", req.URL.Scheme)
			}
			if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return errors.New("HTTPS-подписка перенаправлена на HTTP")
			}
			if len(via) >= 10 {
				return errors.New("слишком много перенаправлений")
			}
			return nil
		},
	}}
}

// Fetch downloads a subscription URL.
func (c *Client) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("пустой URL подписки")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.New("некорректный URL подписки")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("поддерживаются только http и https, получено %q", u.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("некорректный URL подписки")
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return nil, fmt.Errorf("запрос подписки: %s: %v", redactURL(urlErr.URL), urlErr.Err)
		}
		return nil, fmt.Errorf("запрос подписки: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.CopyN(io.Discard, resp.Body, 4<<10)
		return nil, fmt.Errorf("сервер вернул %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodySize+1))
	if err != nil {
		return nil, fmt.Errorf("чтение подписки: %w", err)
	}
	if len(data) > MaxBodySize {
		return nil, fmt.Errorf("подписка больше %d МБ", MaxBodySize>>20)
	}
	if len(data) == 0 {
		return nil, errors.New("сервер вернул пустой ответ")
	}
	return data, nil
}

// redactURL keeps only the origin of a URL for diagnostics: subscription
// paths and query strings often contain a token.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "<url>"
	}
	origin := u.Scheme + "://" + u.Host
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		origin += "/…"
	}
	return origin
}
