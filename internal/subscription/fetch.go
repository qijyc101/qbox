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

// NewClient returns a client with sensible timeouts.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Fetch downloads a subscription URL.
func (c *Client) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("пустой URL подписки")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("некорректный URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("поддерживаются только http и https, получено %q", u.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
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
