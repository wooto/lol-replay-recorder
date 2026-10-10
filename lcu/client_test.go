package lcu_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wooto/lol-replay-recorder/lcu"
)

func TestConfigurationAuthenticatesWithLocalClient(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "riot" || pass != "local-secret" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path != "/lol-replays/v1/configuration" {
			w.WriteHeader(404)
			return
		}
		fmt.Fprint(w, `{"gameVersion":"26.20.1","isLoggedIn":true,"isReplaysEnabled":true}`)
	}))
	defer server.Close()
	client, err := lcu.New(lcu.Config{BaseURL: server.URL, Token: "local-secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	config, err := client.Configuration(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.GameVersion != "26.20.1" || !config.IsLoggedIn || !config.IsReplaysEnabled {
		t.Fatalf("unexpected configuration: %+v", config)
	}
}

func TestStrictTLSCannotBeDisabledBySuppliedTransport(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"gameVersion":"16.20.1"}`) }))
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	custom := &http.Client{Transport: transport}
	defer custom.CloseIdleConnections()
	client, err := lcu.New(lcu.Config{BaseURL: server.URL, Token: "secret", StrictTLS: true, HTTPClient: custom})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.Configuration(context.Background()); err == nil {
		t.Fatal("strict TLS must reject an untrusted certificate")
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("supplied transport was mutated")
	}
}

func TestCheckingReplayStartsDownloadWhenFound(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "KR-123.rofl")
	if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	var first atomic.Bool
	first.Store(true)
	var downloaded atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-replays/v1/metadata/123":
			if first.Swap(false) {
				fmt.Fprint(w, `{"gameId":123,"state":"checking"}`)
			} else if !downloaded.Load() {
				fmt.Fprint(w, `{"gameId":123,"state":"download"}`)
			} else {
				fmt.Fprint(w, `{"gameId":123,"state":"watch"}`)
			}
		case "/lol-replays/v1/rofls/123/download":
			downloaded.Store(true)
			w.WriteHeader(204)
		case "/riotclient/region-locale":
			fmt.Fprint(w, `{"region":"KR"}`)
		case "/lol-replays/v1/rofls/path":
			_ = json.NewEncoder(w).Encode(dir)
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
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	replay, err := client.DownloadReplay(ctx, 123, time.Millisecond)
	if err != nil || !downloaded.Load() || replay.Path != file {
		t.Fatalf("checking -> download -> watch flow failed: %v", err)
	}
}

func TestMetadataPreparationBeforeDownload(t *testing.T) {
	var created atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-replays/v2/metadata/123/create" || r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		var body struct {
			GameVersion string `json:"gameVersion"`
			GameType    string `json:"gameType"`
			QueueID     int    `json:"queueId"`
			GameEnd     int64  `json:"gameEnd"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.GameVersion != "16.20.1" || body.GameType != "MATCHED_GAME" || body.QueueID != 420 || body.GameEnd != 1700000000000 {
			w.WriteHeader(400)
			return
		}
		created.Store(true)
		w.WriteHeader(204)
	}))
	defer server.Close()
	client, err := lcu.New(lcu.Config{BaseURL: server.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	err = client.CreateReplayMetadata(context.Background(), 123, lcu.MetadataRequest{GameVersion: "16.20.1", GameType: "MATCHED_GAME", QueueID: 420, GameEnd: 1700000000000})
	if err != nil || !created.Load() {
		t.Fatalf("metadata creation failed: %v", err)
	}
}

func TestExistingDownloadIsNotRestarted(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "KR-123.rofl")
	if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	var first atomic.Bool
	first.Store(true)
	var restarted atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-replays/v1/metadata/123":
			if first.Swap(false) {
				fmt.Fprint(w, `{"gameId":123,"state":"downloading"}`)
			} else {
				fmt.Fprint(w, `{"gameId":123,"state":"watch"}`)
			}
		case "/lol-replays/v1/rofls/123/download":
			restarted.Store(true)
			w.WriteHeader(409)
		case "/riotclient/region-locale":
			fmt.Fprint(w, `{"region":"KR"}`)
		case "/lol-replays/v1/rofls/path":
			_ = json.NewEncoder(w).Encode(dir)
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
	replay, err := client.DownloadReplay(context.Background(), 123, time.Millisecond)
	if err != nil || restarted.Load() || replay.Path != file {
		t.Fatalf("existing download must finish without restart: %v", err)
	}
}

func TestLaunchSpectatorWithPUUIDAndSpectatorKey(t *testing.T) {
	var launched atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lol-replays/v1/configuration" {
			fmt.Fprint(w, `{"isLoggedIn":true,"isReplaysEnabled":true}`)
			return
		}
		if r.URL.Path != "/lol-spectator/v1/spectate/launch" || r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["puuid"] != "target-id" || body["spectatorKey"] != "handoff-key" {
			w.WriteHeader(400)
			return
		}
		launched.Store(true)
		w.WriteHeader(204)
	}))
	defer server.Close()
	client, err := lcu.New(lcu.Config{BaseURL: server.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	err = client.Spectate(context.Background(), lcu.SpectateRequest{PUUID: "target-id", SpectatorKey: "handoff-key"})
	if err != nil || !launched.Load() {
		t.Fatalf("spectator launch failed: %v", err)
	}
}

func TestCurrentPlayerMatchHistory(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-summoner/v1/current-summoner":
			fmt.Fprint(w, `{"puuid":"player-id","gameName":"선수","tagLine":"KR1","summonerId":7}`)
		case "/lol-match-history/v1/products/lol/player-id/matches":
			if r.URL.Query().Get("begIndex") != "0" || r.URL.Query().Get("endIndex") != "1" {
				w.WriteHeader(400)
				return
			}
			fmt.Fprint(w, `{"games":{"gameCount":1,"games":[{"gameId":123,"gameVersion":"26.20.1","queueId":420}]}}`)
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
	player, err := client.CurrentSummoner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	matches, err := client.MatchHistory(context.Background(), player.PUUID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if player.GameName != "선수" || len(matches.Games.Games) != 1 || matches.Games.Games[0].GameID != 123 {
		t.Fatal("player history was not decoded")
	}
}

func TestWatchRequiresReadyReplayAndIdleClient(t *testing.T) {
	var busy atomic.Bool
	var launched atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-replays/v1/configuration":
			fmt.Fprintf(w, `{"gameVersion":"26.20.1","isLoggedIn":true,"isReplaysEnabled":true,"isPlayingGame":%t}`, busy.Load())
		case "/lol-replays/v1/metadata/123":
			fmt.Fprint(w, `{"gameId":123,"state":"watch"}`)
		case "/lol-replays/v1/rofls/123/watch":
			var body map[string]string
			if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || body["componentType"] != "replay-button_match-history" {
				w.WriteHeader(400)
				return
			}
			launched.Store(true)
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
	if err = client.Watch(context.Background(), 123); err != nil || !launched.Load() {
		t.Fatalf("watch failed: %v", err)
	}
	launched.Store(false)
	busy.Store(true)
	if err = client.Watch(context.Background(), 123); err == nil || launched.Load() {
		t.Fatal("busy client must not launch")
	}
}

func TestDownloadWaitsForVerifiedReplayFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "KR-123.rofl")
	var downloading atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-replays/v1/rofls/123/download":
			var body struct {
				ComponentType string `json:"componentType"`
			}
			if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || body.ComponentType != "replay-button_match-history" {
				w.WriteHeader(400)
				return
			}
			downloading.Store(true)
			w.WriteHeader(204)
		case "/lol-replays/v1/metadata/123":
			if !downloading.Load() {
				fmt.Fprint(w, `{"gameId":123,"state":"download"}`)
				return
			}
			if err := os.WriteFile(file, []byte("replay-fixture"), 0600); err != nil {
				t.Error(err)
			}
			fmt.Fprint(w, `{"gameId":123,"state":"watch"}`)
		case "/lol-replays/v1/rofls/path":
			_ = json.NewEncoder(w).Encode(dir)
		case "/riotclient/region-locale":
			fmt.Fprint(w, `{"region":"KR","locale":"ko_KR"}`)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	replay, err := client.DownloadReplay(ctx, 123, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Path != file || replay.GameID != 123 {
		t.Fatalf("unexpected replay: %+v", replay)
	}
}

func TestLockfileConnectsWithoutExposingPassword(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"gameVersion":"26.20.1","isLoggedIn":true}`)
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "lockfile")
	port := strings.TrimPrefix(server.URL, "https://127.0.0.1:")
	if err := os.WriteFile(file, []byte("LeagueClient:123:"+port+":secret:https"), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := lcu.FromLockfile(file)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.Configuration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, []byte("LeagueClient:123:bad:secret:https"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = lcu.FromLockfile(file)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("expected redacted invalid-lockfile error")
	}
}
