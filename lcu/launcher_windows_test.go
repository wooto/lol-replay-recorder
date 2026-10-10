//go:build windows

package lcu_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wooto/lol-replay-recorder/lcu"
)

func TestOwnedGameHelper(t *testing.T) {
	if os.Getenv("LCU_OWNED_GAME_HELPER") == "1" {
		for {
			time.Sleep(time.Second)
		}
	}
}
func TestLCULaunchOwnsOnlyNewGameProcess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	gameExe := filepath.Join(dir, "League of Legends.exe")
	if err = os.WriteFile(gameExe, data, 0700); err != nil {
		t.Fatal(err)
	}
	replayPath := filepath.Join(dir, "KR-123.rofl")
	if err = os.WriteFile(replayPath, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var child *exec.Cmd
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		if child != nil && child.Process != nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/riotclient/region-locale":
			fmt.Fprint(w, `{"region":"KR"}`)
		case "/lol-replays/v1/rofls/path":
			_ = json.NewEncoder(w).Encode(dir)
		case "/lol-replays/v1/configuration":
			fmt.Fprint(w, `{"isLoggedIn":true,"isReplaysEnabled":true}`)
		case "/lol-replays/v1/metadata/123":
			fmt.Fprint(w, `{"gameId":123,"state":"watch"}`)
		case "/lol-replays/v1/rofls/123/watch":
			child = exec.Command(gameExe, "-test.run=^TestOwnedGameHelper$")
			child.Env = append(os.Environ(), "LCU_OWNED_GAME_HELPER=1")
			if err := child.Start(); err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			w.WriteHeader(204)
		case "/replay/game":
			if child == nil {
				w.WriteHeader(404)
				return
			}
			fmt.Fprintf(w, `{"processID":%d}`, child.Process.Pid)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, err := lcu.New(lcu.Config{BaseURL: server.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	owned, err := client.LaunchReplay(ctx, lcu.Replay{GameID: 123, Path: replayPath}, lcu.LaunchConfig{ReplayURL: server.URL, PollInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	pid := child.Process.Pid
	mu.Unlock()
	if owned.PID() != pid || owned.Exited() {
		t.Fatal("new game was not owned")
	}
	if err = owned.Close(); err != nil {
		t.Fatal(err)
	}
	if !owned.Exited() {
		t.Fatal("owned process did not exit")
	}
	if err = owned.Close(); err != nil {
		t.Fatal("close must be idempotent")
	}
}

func TestLaunchRejectsForeignReplayPathBeforeWatch(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(t.TempDir(), "KR-123.rofl")
	if err := os.WriteFile(foreign, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	launched := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/riotclient/region-locale":
			fmt.Fprint(w, `{"region":"KR"}`)
		case "/lol-replays/v1/rofls/path":
			_ = json.NewEncoder(w).Encode(dir)
		case "/lol-replays/v1/configuration":
			fmt.Fprint(w, `{"isLoggedIn":true,"isReplaysEnabled":true}`)
		case "/lol-replays/v1/metadata/123":
			fmt.Fprint(w, `{"gameId":123,"state":"watch"}`)
		case "/lol-replays/v1/rofls/123/watch":
			launched = true
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, err := lcu.New(lcu.Config{BaseURL: server.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.LaunchReplay(context.Background(), lcu.Replay{GameID: 123, Path: foreign}, lcu.LaunchConfig{ReplayURL: server.URL, Timeout: 30 * time.Millisecond})
	if !errors.Is(err, lcu.ErrReplayFile) || launched {
		t.Fatalf("foreign replay must be rejected before watch: %v", err)
	}
}
