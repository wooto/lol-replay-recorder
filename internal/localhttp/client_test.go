package localhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("global transport must not handle local credentials")
}

func TestCustomDefaultTransportDoesNotInterceptLocalRequests(t *testing.T) {
	previous := http.DefaultTransport
	http.DefaultTransport = failingTransport{}
	t.Cleanup(func() { http.DefaultTransport = previous })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Token: "secret"}, "", true)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var output struct {
		OK bool `json:"ok"`
	}
	if err = client.Request(context.Background(), "GET", "/", nil, &output); err != nil || !output.OK {
		t.Fatalf("isolated local request: output=%+v error=%v", output, err)
	}
}

func TestCancellationDuringResponseReadPreservesContextError(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- client.Request(ctx, "GET", "/", nil, new(any)) }()
	select {
	case <-started:
		cancel()
	case <-ctx.Done():
		t.Fatal("response did not start")
	}
	if err = <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation identity: %v", err)
	}
}

func TestClientTimeoutPreservesDeadlineIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, HTTPClient: &http.Client{Timeout: 30 * time.Millisecond}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err = client.Request(context.Background(), "GET", "/", nil, new(any)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost transport deadline identity: %v", err)
	}
}
