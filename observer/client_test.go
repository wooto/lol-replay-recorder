package observer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCapturePreservesPayloadsAndRetriesLive404(t *testing.T) {
	game := Game{PlatformID: "KR", GameID: 42}
	var chunkTwoRequests atomic.Int32
	var lastInfoRequests atomic.Int32
	metadataLive := metadataJSON(t, game, false, 2, 1, 0, 0)
	metadataEnded := metadataJSON(t, game, true, 2, 1, 2, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/version"):
			_, _ = w.Write([]byte("2.0.0"))
		case strings.Contains(r.URL.Path, "/getGameMetaData/"):
			if lastInfoRequests.Load() == 0 {
				_, _ = w.Write(metadataLive)
			} else {
				_, _ = w.Write(metadataEnded)
			}
		case strings.Contains(r.URL.Path, "/getLastChunkInfo/"):
			count := lastInfoRequests.Add(1)
			info := ChunkInfo{ChunkID: 2, KeyFrameID: 1, NextChunkID: 2, StartGameChunkID: 1, Duration: 30_000, NextAvailableChunk: 100}
			if count > 1 {
				info.EndGameChunkID = 2
			}
			writeJSON(t, w, info)
		case strings.Contains(r.URL.Path, "/getGameDataChunk/"):
			id := lastPathID(t, r.URL.Path, 2)
			if id == 2 && chunkTwoRequests.Add(1) == 1 {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(fmt.Sprintf("raw-chunk-%d\x00", id)))
		case strings.Contains(r.URL.Path, "/getKeyFrame/"):
			_, _ = w.Write([]byte("raw-keyframe-1\x00"))
		case strings.Contains(r.URL.Path, "/endOfGameStats/"):
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, time.Millisecond)
	destination := filepath.Join(t.TempDir(), "capture")
	archive, err := client.Capture(context.Background(), game, destination)
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	manifest := archive.Manifest()
	if !manifest.Complete || len(manifest.ChunkIDs) != 2 || len(manifest.KeyFrameIDs) != 1 || manifest.LastChunk.EndGameChunkID != 2 {
		t.Fatalf("unexpected completed manifest: %+v", manifest)
	}
	if got := chunkTwoRequests.Load(); got < 2 {
		t.Fatalf("chunk 2 requests = %d, want at least 2 after live 404", got)
	}
	for id := uint64(1); id <= 2; id++ {
		got, err := archive.ReadChunk(id)
		if err != nil {
			t.Fatalf("ReadChunk(%d): %v", id, err)
		}
		want := []byte(fmt.Sprintf("raw-chunk-%d\x00", id))
		if string(got) != string(want) {
			t.Fatalf("ReadChunk(%d) = %q, want %q", id, got, want)
		}
	}
	keyframe, err := archive.ReadKeyFrame(1)
	if err != nil || string(keyframe) != "raw-keyframe-1\x00" {
		t.Fatalf("ReadKeyFrame(1) = %q, %v", keyframe, err)
	}
	if got := string(archive.Metadata()); got != string(metadataEnded) {
		t.Fatalf("metadata was not preserved: got %s, want %s", got, metadataEnded)
	}
	reopened, err := OpenArchive(destination)
	if err != nil {
		t.Fatalf("OpenArchive(): %v", err)
	}
	if !reopened.Manifest().Complete || reopened.Version() != "2.0.0" {
		t.Fatalf("reopened archive has wrong completion/version: %+v / %q", reopened.Manifest(), reopened.Version())
	}
}

func TestCaptureEndGapReturnsIncompleteArchive(t *testing.T) {
	game := Game{PlatformID: "KR", GameID: 43}
	metadata := metadataJSON(t, game, true, 2, 1, 2, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/version"):
			_, _ = w.Write([]byte("2.0.0"))
		case strings.Contains(r.URL.Path, "/getGameMetaData/"):
			_, _ = w.Write(metadata)
		case strings.Contains(r.URL.Path, "/getLastChunkInfo/"):
			writeJSON(t, w, ChunkInfo{ChunkID: 2, KeyFrameID: 1, EndGameChunkID: 2})
		case strings.Contains(r.URL.Path, "/getGameDataChunk/"):
			id := lastPathID(t, r.URL.Path, 2)
			if id == 2 {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte("chunk-one"))
		case strings.Contains(r.URL.Path, "/getKeyFrame/"):
			_, _ = w.Write([]byte("keyframe-one"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "partial")
	archive, err := newTestClient(t, server.URL, 0).Capture(context.Background(), game, destination)
	if !errors.Is(err, ErrArchiveIncomplete) {
		t.Fatalf("Capture() error = %v, want ErrArchiveIncomplete", err)
	}
	if archive == nil || archive.Manifest().Complete {
		t.Fatalf("missing end chunk produced a complete archive: %#v", archive)
	}
	if got := archive.Manifest().ChunkIDs; len(got) != 1 || got[0] != 1 {
		t.Fatalf("partial chunk IDs = %v, want [1]", got)
	}
	if _, err := OpenArchive(destination); err != nil {
		t.Fatalf("partial archive should remain readable: %v", err)
	}
}

func TestCaptureCancellationReturnsOpenablePartialArchive(t *testing.T) {
	game := Game{PlatformID: "KR", GameID: 44}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	metadata := metadataJSON(t, game, false, 1, 0, 0, 0)
	var timer *time.Timer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/version"):
			_, _ = w.Write([]byte("2.0.0"))
		case strings.Contains(r.URL.Path, "/getGameMetaData/"):
			_, _ = w.Write(metadata)
		case strings.Contains(r.URL.Path, "/getLastChunkInfo/"):
			writeJSON(t, w, ChunkInfo{ChunkID: 1})
		case strings.Contains(r.URL.Path, "/getGameDataChunk/"):
			_, _ = w.Write([]byte("partial-chunk"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			timer = time.AfterFunc(20*time.Millisecond, cancel)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "cancelled")
	archive, err := newTestClient(t, server.URL, time.Minute).Capture(ctx, game, destination)
	if timer != nil {
		timer.Stop()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Capture() error = %v, want context.Canceled", err)
	}
	if archive == nil || archive.Manifest().Complete {
		t.Fatalf("cancelled capture did not return an incomplete archive: %#v", archive)
	}
	if got := archive.Manifest().ChunkIDs; len(got) != 1 || got[0] != 1 {
		t.Fatalf("persisted partial chunks = %v, want [1]", got)
	}
	if _, err := OpenArchive(destination); err != nil {
		t.Fatalf("cancelled archive should be readable: %v", err)
	}
}

func TestCaptureDoesNotOverwriteExistingDestination(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestClient(t, server.URL, 0)
	destination := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(destination, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive, err := client.Capture(context.Background(), Game{PlatformID: "KR", GameID: 45}, destination)
	if archive != nil || !errors.Is(err, ErrOutputExists) {
		t.Fatalf("Capture() = %v, %v; want nil, ErrOutputExists", archive, err)
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "keep" {
		t.Fatalf("existing destination changed: %q, %v", got, err)
	}
}

func TestCaptureRejectsMismatchedGameKeyAndMalformedLastChunkInfo(t *testing.T) {
	t.Run("mismatched game key", func(t *testing.T) {
		requested := Game{PlatformID: "KR", GameID: 46}
		wrong := metadataJSON(t, Game{PlatformID: "KR", GameID: 47}, false, 0, 0, 0, 0)
		var lastInfoCalls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/version"):
				_, _ = w.Write([]byte("2.0.0"))
			case strings.Contains(r.URL.Path, "/getGameMetaData/"):
				_, _ = w.Write(wrong)
			case strings.Contains(r.URL.Path, "/getLastChunkInfo/"):
				lastInfoCalls.Add(1)
				writeJSON(t, w, ChunkInfo{ChunkID: 0})
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()
		directory := filepath.Join(t.TempDir(), "mismatch")
		archive, err := newTestClient(t, server.URL, 0).Capture(context.Background(), requested, directory)
		if err == nil || archive == nil || archive.Manifest().Complete {
			t.Fatalf("Capture() = %v, %v; want incomplete archive and metadata error", archive, err)
		}
		if lastInfoCalls.Load() != 0 {
			t.Fatalf("requested last chunk info %d times after mismatched gameKey", lastInfoCalls.Load())
		}
	})

	t.Run("error object instead of chunk info", func(t *testing.T) {
		game := Game{PlatformID: "KR", GameID: 48}
		metadata := metadataJSON(t, game, false, 0, 0, 0, 0)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/version"):
				_, _ = w.Write([]byte("2.0.0"))
			case strings.Contains(r.URL.Path, "/getGameMetaData/"):
				_, _ = w.Write(metadata)
			case strings.Contains(r.URL.Path, "/getLastChunkInfo/"):
				_, _ = w.Write([]byte(`{"error":"metadata not initialized"}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()
		directory := filepath.Join(t.TempDir(), "bad-info")
		archive, err := newTestClient(t, server.URL, 0).Capture(context.Background(), game, directory)
		if err == nil || archive == nil || archive.Manifest().Complete {
			t.Fatalf("Capture() = %v, %v; want incomplete archive and chunk-info error", archive, err)
		}
		if _, err := OpenArchive(directory); err != nil {
			t.Fatalf("failed capture did not retain a readable partial archive: %v", err)
		}
	})
}

func TestCaptureRejectsConflictingExplicitEndMarkers(t *testing.T) {
	game := Game{PlatformID: "KR", GameID: 49}
	metadata := metadataJSON(t, game, true, 2, 1, 2, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/version"):
			_, _ = w.Write([]byte("2.0.0"))
		case strings.Contains(r.URL.Path, "/getGameMetaData/"):
			_, _ = w.Write(metadata)
		case strings.Contains(r.URL.Path, "/getLastChunkInfo/"):
			writeJSON(t, w, ChunkInfo{ChunkID: 2, KeyFrameID: 1, EndGameChunkID: 3})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	directory := filepath.Join(t.TempDir(), "conflicting-end")
	archive, err := newTestClient(t, server.URL, 0).Capture(context.Background(), game, directory)
	if !errors.Is(err, ErrArchiveIncomplete) {
		t.Fatalf("Capture() error = %v, want ErrArchiveIncomplete", err)
	}
	if archive == nil || archive.Manifest().Complete {
		t.Fatalf("conflicting end IDs produced a complete archive: %#v", archive)
	}
}

func TestCaptureRejectsNullMetadataFields(t *testing.T) {
	for _, field := range []string{"gameEnded", "lastChunkId", "lastKeyFrameId", "endGameChunkId", "endGameKeyFrameId"} {
		t.Run(field, func(t *testing.T) {
			game := Game{PlatformID: "KR", GameID: 50}
			metadata := []byte(fmt.Sprintf(`{"gameKey":{"platformId":"KR","gameId":50},"%s":null}`, field))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/version"):
					_, _ = w.Write([]byte("2.0.0"))
				case strings.Contains(r.URL.Path, "/getGameMetaData/"):
					_, _ = w.Write(metadata)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			directory := filepath.Join(t.TempDir(), "null-metadata")
			archive, err := newTestClient(t, server.URL, 0).Capture(context.Background(), game, directory)
			if err == nil || archive == nil || archive.Manifest().Complete {
				t.Fatalf("Capture() = %v, %v; want an incomplete archive and metadata error", archive, err)
			}
		})
	}
}

func TestNewClientRejectsUnsafeURLsAndBlocksRedirects(t *testing.T) {
	for _, raw := range []string{
		"", "ftp://observer.example", "http://user:pass@observer.example", "http://observer.example?x=1",
		"http://observer.example#fragment", "http://observer.example/a/../b",
	} {
		if _, err := NewClient(ClientConfig{BaseURL: raw}); err == nil {
			t.Errorf("NewClient(%q) succeeded; want validation error", raw)
		}
	}
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetCalls.Add(1)
		_, _ = w.Write([]byte("should not be followed"))
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/stolen", http.StatusFound)
	}))
	defer redirect.Close()
	client := newTestClient(t, redirect.URL, 0)
	_, err := client.fetchVersion(context.Background())
	if err == nil || targetCalls.Load() != 0 {
		t.Fatalf("redirect was followed or accepted: target calls %d, error %v", targetCalls.Load(), err)
	}
}

func newTestClient(t *testing.T, base string, poll time.Duration) *Client {
	t.Helper()
	client, err := NewClient(ClientConfig{BaseURL: base, PollInterval: poll})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func metadataJSON(t *testing.T, game Game, ended bool, lastChunk, lastKeyFrame, endChunk, endKeyFrame uint64) []byte {
	t.Helper()
	value := map[string]any{
		"gameKey":   map[string]any{"platformId": game.PlatformID, "gameId": game.GameID},
		"gameEnded": ended, "lastChunkId": lastChunk, "lastKeyFrameId": lastKeyFrame,
		"endGameChunkId": endChunk, "endGameKeyFrameId": endKeyFrame,
		"chunkTimeInterval": 30_000, "pendingAvailableChunkInfo": []any{}, "pendingAvailableKeyFrameInfo": []any{},
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode fixture response: %v", err)
	}
}

func lastPathID(t *testing.T, path string, fromEnd int) uint64 {
	t.Helper()
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < fromEnd {
		t.Fatalf("path has too few segments: %s", path)
	}
	id, err := strconv.ParseUint(parts[len(parts)-fromEnd], 10, 64)
	if err != nil {
		t.Fatalf("parse ID from %s: %v", path, err)
	}
	return id
}
