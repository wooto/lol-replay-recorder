package discovery

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testAPIKey = "test-secret-token"

func newTestClient(t *testing.T, serverURL string, client *http.Client) *Client {
	t.Helper()
	result, err := New(Config{
		APIKey:          testAPIKey,
		HTTPClient:      client,
		AccountBaseURL:  serverURL,
		PlatformBaseURL: serverURL,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return result
}

func TestLookupEscapesRiotIDAndSendsTokenInHeader(t *testing.T) {
	gameName := "Ahri # 雪/狐"
	tagLine := "KR #1"
	expectedPath := "/riot/account/v1/accounts/by-riot-id/" + url.PathEscape(gameName) + "/" + url.PathEscape(tagLine)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if got := r.URL.EscapedPath(); got != expectedPath {
			t.Errorf("escaped path = %q, want %q", got, expectedPath)
		}
		if got := r.Header.Get("X-Riot-Token"); got != testAPIKey {
			t.Errorf("X-Riot-Token = %q, want configured token", got)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"puuid":"puuid-123"}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, nil)
	got, err := client.Lookup(context.Background(), gameName, tagLine)
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if got.PUUID != "puuid-123" {
		t.Fatalf("Lookup() = %+v, want PUUID puuid-123", got)
	}
}

func TestActiveGameDecodesObserverHandoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.EscapedPath(), "/lol/spectator/v5/active-games/by-summoner/puuid-1"; got != want {
			t.Errorf("escaped path = %q, want %q", got, want)
		}
		if got := r.Header.Get("X-Riot-Token"); got != testAPIKey {
			t.Errorf("X-Riot-Token = %q, want configured token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gameId":987654321,"platformId":"KR","gameStartTime":1720000000000,"observers":{"encryptionKey":"observer-secret"}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, nil)
	got, err := client.ActiveGame(context.Background(), "puuid-1")
	if err != nil {
		t.Fatalf("ActiveGame() error = %v", err)
	}
	if got.GameID != 987654321 || got.PlatformID != "KR" || got.GameStartTime != 1720000000000 {
		t.Fatal("ActiveGame() returned unexpected game metadata")
	}
	if got.EncryptionKey != "observer-secret" {
		t.Fatal("ActiveGame() returned an unexpected observer key")
	}
}

func TestLookupEnforcesResponseSizeLimit(t *testing.T) {
	payload := `{"puuid":"puuid-123"}`
	for _, tc := range []struct {
		name string
		body string
		want error
	}{
		{name: "exact limit is accepted", body: payload + strings.Repeat(" ", maxResponseBytes-len(payload))},
		{name: "oversized trailing whitespace is rejected", body: payload + strings.Repeat(" ", maxResponseBytes+1-len(payload)), want: ErrInvalidResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client := newTestClient(t, server.URL, nil)

			account, err := client.Lookup(context.Background(), "name", "tag")
			if tc.want != nil {
				if !errors.Is(err, tc.want) {
					t.Fatalf("Lookup() error = %v, want ErrInvalidResponse", err)
				}
				return
			}
			if err != nil || account.PUUID != "puuid-123" {
				t.Fatal("Lookup() failed to accept a valid response at the size limit")
			}
		})
	}
}

func Test404ResponsesAreDistinct(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, nil)

	_, err := client.ActiveGame(context.Background(), "puuid-1")
	if !errors.Is(err, ErrNoActiveGame) {
		t.Fatalf("ActiveGame() error = %v, want ErrNoActiveGame", err)
	}
	var status *StatusError
	if errors.As(err, &status) {
		t.Fatalf("active-game 404 should map to sentinel, got status error %+v", status)
	}

	_, err = client.Lookup(context.Background(), "name", "tag")
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("Lookup() error = %v, want ErrAccountNotFound", err)
	}
	if errors.Is(err, ErrNoActiveGame) {
		t.Fatalf("account 404 must not map to ErrNoActiveGame: %v", err)
	}
	if !errors.As(err, &status) || status.StatusCode != http.StatusNotFound {
		t.Fatalf("Lookup() error should retain HTTP 404 status, got %v", err)
	}
}

func TestWaitForGameHonorsRetryAfterAndContext(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "1")
		http.Error(w, "private response body", http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := client.WaitForGame(ctx, "puuid-1", 5*time.Millisecond)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForGame() error = %v, want context deadline", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("request count = %d, want 1 (must wait for Retry-After)", got)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("context cancellation took too long: %s", elapsed)
	}

	_, err = client.ActiveGame(context.Background(), "puuid-1")
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusTooManyRequests || status.RetryAfter < time.Second {
		t.Fatalf("429 status = %#v, want code 429 and one-second delay", status)
	}
	if strings.Contains(err.Error(), testAPIKey) || strings.Contains(err.Error(), "private response body") {
		t.Fatalf("status error leaked a secret or response body: %v", err)
	}
}

func TestWaitForGameReturnsAuthorizationErrorsImmediately(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, nil)

	_, err := client.WaitForGame(context.Background(), "puuid-1", time.Millisecond)
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusForbidden {
		t.Fatalf("WaitForGame() error = %v, want HTTP 403 status", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("request count = %d, want 1", got)
	}
}

func TestWaitForGameCancellationInterruptsNoGamePolling(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.NotFound(w, nil)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := client.WaitForGame(ctx, "puuid-1", time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForGame() error = %v, want context deadline", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("request count = %d, want 1", got)
	}
}

func TestRequestHonorsContextCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := client.Lookup(ctx, "name", "tag")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Lookup() error = %v, want context deadline", err)
	}
	select {
	case <-requestStarted:
	default:
		t.Fatal("server did not receive the request")
	}
}

func TestRejectsRedirectWithoutForwardingToken(t *testing.T) {
	var destinationRequests atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationRequests.Add(1)
		if got := r.Header.Get("X-Riot-Token"); got != "" {
			t.Errorf("redirect forwarded X-Riot-Token %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"puuid":"unexpected"}`))
	}))
	defer destination.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", destination.URL+"/collect")
		w.WriteHeader(http.StatusFound)
	}))
	defer source.Close()

	allowRedirects := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return nil }}
	client := newTestClient(t, source.URL, allowRedirects)
	_, err := client.Lookup(context.Background(), "name", "tag")
	if !errors.Is(err, ErrRedirect) {
		t.Fatalf("Lookup() error = %v, want ErrRedirect", err)
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Fatalf("redirect error leaked API key: %v", err)
	}
	if got := destinationRequests.Load(); got != 0 {
		t.Fatalf("destination request count = %d, want 0", got)
	}
}

func TestRejectsInvalidBaseURLs(t *testing.T) {
	for _, base := range []string{
		"http://api.example.invalid",
		"https://user:pass@example.invalid",
		"https://example.invalid?token=secret",
		"https://example.invalid#fragment",
	} {
		t.Run(base, func(t *testing.T) {
			_, err := New(Config{APIKey: testAPIKey, AccountBaseURL: base})
			if err == nil {
				t.Fatalf("New() accepted invalid base URL %q", base)
			}
			if strings.Contains(err.Error(), testAPIKey) {
				t.Fatalf("configuration error leaked API key: %v", err)
			}
		})
	}
}

func TestRejectsEmptyInputsAndMalformedSuccessfulResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/account/") {
			_, _ = w.Write([]byte(`{"puuid":"  "}`))
			return
		}
		_, _ = w.Write([]byte(`{"gameId":0,"platformId":"KR","observers":{"encryptionKey":"key"}}`))
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, nil)

	if _, err := client.Lookup(context.Background(), " ", "tag"); err == nil {
		t.Fatal("Lookup() accepted an empty game name")
	}
	if _, err := client.ActiveGame(context.Background(), " "); err == nil {
		t.Fatal("ActiveGame() accepted an empty PUUID")
	}
	if _, err := client.Lookup(context.Background(), "name", "tag"); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("Lookup() error = %v, want ErrInvalidResponse", err)
	}
	if _, err := client.ActiveGame(context.Background(), "puuid"); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("ActiveGame() error = %v, want ErrInvalidResponse", err)
	}
}
