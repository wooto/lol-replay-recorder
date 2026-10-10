package integration_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wooto/lol-replay-recorder/lcu"
	"github.com/wooto/lol-replay-recorder/liveclient"
	"github.com/wooto/lol-replay-recorder/riotclient"
	"github.com/wooto/lol-replay-recorder/spectator"
)

type boundary struct {
	name    string
	connect func(string) (func(context.Context) error, func(), error)
}

func boundaries() []boundary {
	return []boundary{
		{"lcu", func(base string) (func(context.Context) error, func(), error) {
			c, e := lcu.New(lcu.Config{BaseURL: base, Token: "local-secret"})
			if e != nil {
				return nil, nil, e
			}
			return func(ctx context.Context) error { _, e := c.Configuration(ctx); return e }, c.Close, nil
		}},
		{"riotclient", func(base string) (func(context.Context) error, func(), error) {
			c, e := riotclient.New(riotclient.Config{BaseURL: base, Token: "local-secret"})
			if e != nil {
				return nil, nil, e
			}
			return func(ctx context.Context) error { _, e := c.Authorization(ctx); return e }, c.Close, nil
		}},
		{"liveclient", func(base string) (func(context.Context) error, func(), error) {
			c, e := liveclient.New(liveclient.Config{BaseURL: base})
			if e != nil {
				return nil, nil, e
			}
			return func(ctx context.Context) error { _, e := c.AllGameData(ctx); return e }, c.Close, nil
		}},
		{"spectator", func(base string) (func(context.Context) error, func(), error) {
			c, e := spectator.New(spectator.Config{BaseURL: base})
			if e != nil {
				return nil, nil, e
			}
			return func(ctx context.Context) error { _, e := c.Game(ctx); return e }, c.Close, nil
		}},
	}
}
func TestLocalClientsRejectNonLocalAndInvalidOrigins(t *testing.T) {
	for _, b := range boundaries() {
		t.Run(b.name, func(t *testing.T) {
			for _, origin := range []string{"https://example.com", "https://localhost:2999", "https://user:pass@127.0.0.1", "https://127.0.0.1:65536", "https://127.0.0.1:0", "https://127.0.0.1/path", "https://127.0.0.1?token=secret"} {
				_, close, err := b.connect(origin)
				if close != nil {
					close()
				}
				if err == nil {
					t.Errorf("accepted invalid origin %s", origin)
				}
			}
		})
	}
}
func TestLocalClientHTTPFailuresAndRedirects(t *testing.T) {
	var redirected atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer destination.Close()
	for _, b := range boundaries() {
		t.Run(b.name, func(t *testing.T) {
			for _, status := range []int{401, 403, 404, 429, 500, 302} {
				t.Run(fmt.Sprint(status), func(t *testing.T) {
					server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Location", destination.URL)
						w.WriteHeader(status)
						fmt.Fprint(w, "local-secret: server error details")
					}))
					defer server.Close()
					fetch, close, err := b.connect(server.URL)
					if err != nil {
						t.Fatal(err)
					}
					defer close()
					err = fetch(context.Background())
					var httpError *lcu.StatusError
					if !errors.As(err, &httpError) || httpError.StatusCode != status {
						t.Fatalf("expected HTTP %d, got %v", status, err)
					}
					if strings.Contains(err.Error(), "local-secret") {
						t.Fatal("error exposed credentials")
					}
				})
			}
		})
	}
	if redirected.Load() != 0 {
		t.Fatal("local client followed a redirect")
	}
}
func TestCanceledLocalRequestsNeverReachServer(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer server.Close()
	for _, b := range boundaries() {
		t.Run(b.name, func(t *testing.T) {
			fetch, close, err := b.connect(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer close()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err = fetch(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatal("canceled requests reached server")
	}
}
func TestLocalClientsRejectMalformedSuccessResponses(t *testing.T) {
	for _, b := range boundaries() {
		t.Run(b.name, func(t *testing.T) {
			for _, body := range []string{`null`, `{"bad":`, `{}{}`, strings.Repeat("x", (8<<20)+1)} {
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
				fetch, close, err := b.connect(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				if err = fetch(context.Background()); err == nil {
					t.Error("malformed success response was accepted")
				}
				close()
				server.Close()
			}
		})
	}
}
