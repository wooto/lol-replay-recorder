// Package localhttp implements bounded, credential-isolated local JSON requests.
package localhttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BaseURL    string
	Token      string
	StrictTLS  bool
	HTTPClient *http.Client
}
type Client struct {
	base  string
	token string
	http  *http.Client
}
type StatusError struct{ StatusCode int }

func (e *StatusError) Error() string { return fmt.Sprintf("local API returned HTTP %d", e.StatusCode) }

var ErrResponse = errors.New("invalid local API response")

func New(config Config, fallback string, authenticated bool) (*Client, error) {
	base := config.BaseURL
	if base == "" {
		base = fallback
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, errors.New("invalid local API origin")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("local API requires a numeric loopback HTTP(S) origin")
	}
	if authenticated && strings.TrimSpace(config.Token) == "" {
		return nil, errors.New("local client token is required")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, errors.New("invalid local API port")
		}
	}
	transport := CloneTransport()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: !config.StrictTLS} // #nosec G402 -- numeric loopback only; isolated transport; redirects disabled.
	h := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	if config.HTTPClient != nil {
		copy := *config.HTTPClient
		h = &copy
		if supplied, ok := h.Transport.(*http.Transport); ok {
			isolated := supplied.Clone()
			isolated.Proxy = nil
			if isolated.TLSClientConfig == nil {
				isolated.TLSClientConfig = &tls.Config{}
			} else {
				isolated.TLSClientConfig = isolated.TLSClientConfig.Clone()
			}
			isolated.TLSClientConfig.MinVersion = tls.VersionTLS12
			isolated.TLSClientConfig.InsecureSkipVerify = !config.StrictTLS // #nosec G402 -- validated numeric loopback only.
			h.Transport = isolated
		} else if h.Transport == nil {
			h.Transport = transport
		}
		if h.Timeout <= 0 {
			h.Timeout = 10 * time.Second
		}
	}
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	token := ""
	if authenticated {
		token = config.Token
	}
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: h}, nil
}
func (c *Client) Close() { c.http.CloseIdleConnections() }
func (c *Client) Request(ctx context.Context, method, path string, body, out any) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return errors.New("invalid request body")
		}
		input = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, input)
	if err != nil {
		return errors.New("invalid request")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.SetBasicAuth("riot", c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("local API connection timed out: %w", context.DeadlineExceeded)
		}
		return errors.New("local API connection failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &StatusError{StatusCode: resp.StatusCode}
	}
	const limit = 8 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("local API response timed out: %w", context.DeadlineExceeded)
	}
	if err != nil || len(data) > limit {
		return ErrResponse
	}
	if out == nil {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return ErrResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(out) != nil {
		return ErrResponse
	}
	if decoder.Decode(new(any)) != io.EOF {
		return ErrResponse
	}
	return nil
}
