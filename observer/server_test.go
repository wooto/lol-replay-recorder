package observer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

var (
	testGame  = Game{PlatformID: "KR", GameID: 123456789}
	chunkData = map[uint64][]byte{
		1: {0x00, 0x01, 0xff, 'c', '1'},
		2: {0x00, 0x02, 0xfe, 'c', '2'},
	}
	keyFrameData = map[uint64][]byte{
		1: {0x10, 0x11, 0xfd, 'k', '1'},
		2: {0x20, 0x21, 0xfc, 'k', '2'},
	}
	testStats    = []byte(`{"gameId":123456789,"winner":"blue"}`)
	testMetadata = []byte(`{
		"gameKey":{"gameId":123456789,"platformId":"KR"},
		"gameEnded":false,
		"lastChunkId":1,
		"lastKeyFrameId":1,
		"endGameChunkId":0,
		"endGameKeyFrameId":2,
		"chunkTimeInterval":30000,
		"pendingAvailableChunkInfo":[
			{"chunkId":1,"duration":29000,"receivedTime":"source-chunk-time"},
			{"chunkId":99,"duration":30000,"receivedTime":"stale"}
		],
		"pendingAvailableKeyFrameInfo":[
			{"keyFrameId":1,"receivedTime":"source-keyframe-time","nextChunkId":1},
			{"keyFrameId":99,"receivedTime":"stale","nextChunkId":99}
		]
	}`)
	testLastChunk = ChunkInfo{
		ChunkID:            2,
		KeyFrameID:         2,
		NextChunkID:        2,
		EndStartupChunkID:  1,
		StartGameChunkID:   1,
		EndGameChunkID:     2,
		Duration:           30000,
		AvailableSince:     60000,
		NextAvailableChunk: 90000,
	}
)

type archiveFixture struct {
	archive *Archive
	dir     string
}

func newArchiveFixture(t *testing.T, includeStats bool) archiveFixture {
	return newArchiveFixtureWithKeyFrames(t, includeStats, testLastChunk.KeyFrameID)
}

func newArchiveFixtureWithKeyFrames(t *testing.T, includeStats bool, finalKeyFrameID uint64) archiveFixture {
	t.Helper()
	rawMetadata := append([]byte(nil), testMetadata...)
	if finalKeyFrameID == 0 {
		rawMetadata = []byte(strings.ReplaceAll(string(rawMetadata), `"lastKeyFrameId":1`, `"lastKeyFrameId":0`))
		rawMetadata = []byte(strings.ReplaceAll(string(rawMetadata), `"endGameKeyFrameId":2`, `"endGameKeyFrameId":0`))
	}
	return newArchiveFixtureWithRawMetadata(t, includeStats, finalKeyFrameID, rawMetadata)
}

func newArchiveFixtureWithRawMetadata(t *testing.T, includeStats bool, finalKeyFrameID uint64, rawMetadata []byte) archiveFixture {
	t.Helper()
	lastSnapshot := testLastChunk
	lastSnapshot.KeyFrameID = finalKeyFrameID
	lastInfo, err := json.Marshal(lastSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/observer-mode/rest/consumer/")
		switch {
		case path == "version":
			_, _ = io.WriteString(w, "capture-fixture-1.2.3")
		case path == "getGameMetaData/KR/123456789/0/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(rawMetadata)
		case path == "getLastChunkInfo/KR/123456789/0/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(lastInfo)
		case path == "endOfGameStats/KR/123456789/null" && includeStats:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(testStats)
		case strings.HasPrefix(path, "getGameDataChunk/KR/123456789/") && strings.HasSuffix(path, "/token"):
			id, ok := routeID(path, "getGameDataChunk")
			data, exists := chunkData[id]
			if !ok || !exists {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(data)
		case strings.HasPrefix(path, "getKeyFrame/KR/123456789/") && strings.HasSuffix(path, "/token"):
			id, ok := routeID(path, "getKeyFrame")
			data, exists := keyFrameData[id]
			if !ok || !exists {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(ClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "capture")
	archive, err := client.Capture(context.Background(), testGame, dir)
	if err != nil {
		t.Fatalf("Capture fixture: %v", err)
	}
	if !archive.Manifest().Complete {
		t.Fatal("Capture fixture is not complete")
	}
	return archiveFixture{archive: archive, dir: dir}
}

func routeID(path, route string) (uint64, bool) {
	parts := strings.Split(path, "/")
	if len(parts) != 5 || parts[0] != route || parts[1] != "KR" || parts[2] != "123456789" || parts[4] != "token" {
		return 0, false
	}
	id, err := strconv.ParseUint(parts[3], 10, 64)
	return id, err == nil && id != 0
}

func TestNewReplayHandlerServesCompleteArchive(t *testing.T) {
	fixture := newArchiveFixture(t, true)
	handler, err := NewReplayHandler(fixture.archive)
	if err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		path        string
		contentType string
		want        []byte
	}{
		{"version", "text/plain; charset=utf-8", []byte("capture-fixture-1.2.3")},
		{"getGameDataChunk/KR/123456789/1/token", "application/octet-stream", chunkData[1]},
		{"getGameDataChunk/KR/123456789/2/token", "application/octet-stream", chunkData[2]},
		{"getKeyFrame/KR/123456789/1/token", "application/octet-stream", keyFrameData[1]},
		{"getKeyFrame/KR/123456789/2/token", "application/octet-stream", keyFrameData[2]},
		{"endOfGameStats/KR/123456789/null", "application/json", testStats},
	}
	for _, check := range checks {
		t.Run(check.path, func(t *testing.T) {
			response := request(t, handler, http.MethodGet, "/observer-mode/rest/consumer/"+check.path)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
			}
			if got := response.Header().Get("Content-Type"); got != check.contentType {
				t.Fatalf("Content-Type = %q, want %q", got, check.contentType)
			}
			if !bytes.Equal(response.Body.Bytes(), check.want) {
				t.Fatalf("body = %v, want exact bytes %v", response.Body.Bytes(), check.want)
			}
		})
	}

	metadataResponse := request(t, handler, http.MethodGet, "/observer-mode/rest/consumer/getGameMetaData/KR/123456789/0/token")
	if metadataResponse.Code != http.StatusOK {
		t.Fatalf("metadata status = %d", metadataResponse.Code)
	}
	var metadata map[string]any
	if err := json.Unmarshal(metadataResponse.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["gameEnded"] != true || metadata["lastChunkId"] != float64(2) || metadata["lastKeyFrameId"] != float64(2) || metadata["endGameChunkId"] != float64(2) {
		t.Fatalf("metadata final fields were not normalized: %#v", metadata)
	}
	chunks := metadata["pendingAvailableChunkInfo"].([]any)
	if len(chunks) != 2 || chunks[0].(map[string]any)["receivedTime"] != "source-chunk-time" || chunks[1].(map[string]any)["chunkId"] != float64(2) {
		t.Fatalf("chunk availability does not match archive: %#v", chunks)
	}
	keyframes := metadata["pendingAvailableKeyFrameInfo"].([]any)
	if len(keyframes) != 2 {
		t.Fatalf("keyframe availability does not match archive: %#v", keyframes)
	}
	if keyframes[0].(map[string]any)["nextChunkId"] != float64(1) {
		t.Fatalf("source nextChunkId was not preserved: %#v", keyframes[0])
	}
	if _, exists := keyframes[1].(map[string]any)["nextChunkId"]; exists {
		t.Fatalf("handler invented a keyframe nextChunkId: %#v", keyframes[1])
	}

	if bytes.Equal(fixture.archive.Metadata(), metadataResponse.Body.Bytes()) {
		t.Fatal("handler rewrote the archive's raw metadata in place")
	}
	var sourceMetadata map[string]any
	if err := json.Unmarshal(fixture.archive.Metadata(), &sourceMetadata); err != nil {
		t.Fatal(err)
	}
	if sourceMetadata["gameEnded"] != false {
		t.Fatalf("archive raw metadata was modified: %#v", sourceMetadata)
	}

	firstLast := request(t, handler, http.MethodGet, "/observer-mode/rest/consumer/getLastChunkInfo/KR/123456789/0/token")
	secondLast := request(t, handler, http.MethodGet, "/observer-mode/rest/consumer/getLastChunkInfo/KR/123456789/1/token")
	if firstLast.Code != http.StatusOK || !bytes.Equal(firstLast.Body.Bytes(), secondLast.Body.Bytes()) {
		t.Fatalf("last chunk endpoint was not a stable snapshot: %d / %d", firstLast.Code, secondLast.Code)
	}
	var last ChunkInfo
	if err := json.Unmarshal(firstLast.Body.Bytes(), &last); err != nil {
		t.Fatal(err)
	}
	if last.ChunkID != 2 || last.EndGameChunkID != 2 || last.NextAvailableChunk != 0 {
		t.Fatalf("last chunk snapshot = %#v", last)
	}

	head := request(t, handler, http.MethodHead, "/observer-mode/rest/consumer/getGameDataChunk/KR/123456789/1/token")
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != fmt.Sprint(len(chunkData[1])) {
		t.Fatalf("HEAD response status=%d length=%q body=%d", head.Code, head.Header().Get("Content-Length"), head.Body.Len())
	}
}

func TestNewReplayHandlerRejectsIncompleteAndServesMissingAsNotFound(t *testing.T) {
	fixture := newArchiveFixture(t, true)
	partialDir := filepath.Join(t.TempDir(), "partial")
	if err := copyArchiveWithComplete(fixture, partialDir, false); err != nil {
		t.Fatal(err)
	}
	partial, err := OpenArchive(partialDir)
	if err != nil {
		t.Fatalf("open incomplete archive fixture: %v", err)
	}
	if _, err := NewReplayHandler(partial); err == nil {
		t.Fatal("NewReplayHandler accepted an incomplete archive")
	}

	handler, err := NewReplayHandler(fixture.archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(fixture.dir, "chunks", "2.bin")); err != nil {
		t.Fatal(err)
	}
	missing := request(t, handler, http.MethodGet, "/observer-mode/rest/consumer/getGameDataChunk/KR/123456789/2/token")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing payload status = %d, want 404", missing.Code)
	}
	if strings.Contains(missing.Body.String(), fixture.dir) {
		t.Fatalf("archive path leaked in error response: %q", missing.Body.String())
	}

	noStats := newArchiveFixture(t, false)
	noStatsHandler, err := NewReplayHandler(noStats.archive)
	if err != nil {
		t.Fatal(err)
	}
	stats := request(t, noStatsHandler, http.MethodGet, "/observer-mode/rest/consumer/endOfGameStats/KR/123456789/null")
	if stats.Code != http.StatusNotFound {
		t.Fatalf("missing optional stats status = %d, want 404", stats.Code)
	}
}

func TestReplayHandlerAllowsCompleteArchiveWithNoKeyframes(t *testing.T) {
	fixture := newArchiveFixtureWithKeyFrames(t, false, 0)
	if len(fixture.archive.Manifest().KeyFrameIDs) != 0 {
		t.Fatalf("fixture unexpectedly has keyframes: %v", fixture.archive.Manifest().KeyFrameIDs)
	}
	handler, err := NewReplayHandler(fixture.archive)
	if err != nil {
		t.Fatal(err)
	}
	metadata := request(t, handler, http.MethodGet, "/observer-mode/rest/consumer/getGameMetaData/KR/123456789/0/token")
	if metadata.Code != http.StatusOK {
		t.Fatalf("metadata status = %d", metadata.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(metadata.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["lastKeyFrameId"] != float64(0) || body["endGameKeyFrameId"] != float64(0) {
		t.Fatalf("keyframe metadata should remain zero: %#v", body)
	}
	if list := body["pendingAvailableKeyFrameInfo"].([]any); len(list) != 0 {
		t.Fatalf("keyframe availability should be empty: %#v", list)
	}
}

func TestNewReplayHandlerRejectsMalformedAvailabilityIDs(t *testing.T) {
	cases := []struct {
		name    string
		field   string
		entries string
	}{
		{"null chunk ID", "pendingAvailableChunkInfo", `[{"chunkId":null}]`},
		{"missing chunk ID", "pendingAvailableChunkInfo", `[{"duration":30000}]`},
		{"string chunk ID", "pendingAvailableChunkInfo", `[{"chunkId":"1"}]`},
		{"zero chunk ID", "pendingAvailableChunkInfo", `[{"chunkId":0}]`},
		{"oversized chunk ID", "pendingAvailableChunkInfo", `[{"chunkId":100001}]`},
		{"duplicate chunk ID", "pendingAvailableChunkInfo", `[{"chunkId":1},{"chunkId":1}]`},
		{"null keyframe ID", "pendingAvailableKeyFrameInfo", `[{"keyFrameId":null}]`},
		{"missing keyframe ID", "pendingAvailableKeyFrameInfo", `[{"nextChunkId":1}]`},
		{"string keyframe ID", "pendingAvailableKeyFrameInfo", `[{"keyFrameId":"1"}]`},
		{"zero keyframe ID", "pendingAvailableKeyFrameInfo", `[{"keyFrameId":0}]`},
		{"oversized keyframe ID", "pendingAvailableKeyFrameInfo", `[{"keyFrameId":100001}]`},
		{"duplicate keyframe ID", "pendingAvailableKeyFrameInfo", `[{"keyFrameId":1},{"keyFrameId":1}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rawMetadata := metadataWithAvailability(t, tc.field, tc.entries)
			fixture := newArchiveFixtureWithRawMetadata(t, false, testLastChunk.KeyFrameID, rawMetadata)
			if _, err := NewReplayHandler(fixture.archive); err == nil {
				t.Fatal("NewReplayHandler accepted malformed availability IDs")
			}
		})
	}
}

func TestReplayHandlerRejectsWrongIdentityPathsAndMethods(t *testing.T) {
	fixture := newArchiveFixture(t, true)
	handler, err := NewReplayHandler(fixture.archive)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/observer-mode/rest/consumer/getGameMetaData/EUW1/123456789/0/token",
		"/observer-mode/rest/consumer/getGameMetaData/KR/123456788/0/token",
		"/observer-mode/rest/consumer/getGameMetaData/KR/123456789/2/token",
		"/observer-mode/rest/consumer/getGameMetaData/KR/123456789/0/not-token",
		"/observer-mode/rest/consumer/getGameDataChunk/KR/123456789/0/token",
		"/observer-mode/rest/consumer/getGameDataChunk/KR/123456789/abc/token",
		"/observer-mode/rest/consumer/getGameDataChunk/KR/123456789/../token",
		"/observer-mode/rest/consumer/getGameDataChunk/KR/123456789/%2e%2e/token",
		"/observer-mode/rest/consumer/endOfGameStats/KR/123456789/../../manifest.json",
		"/observer-mode/rest/consumer/unknown",
		"/outside/getGameDataChunk/KR/123456789/1/token",
	} {
		response := request(t, handler, http.MethodGet, path)
		if response.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404", path, response.Code)
		}
	}
	post := request(t, handler, http.MethodPost, "/observer-mode/rest/consumer/version")
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST status=%d Allow=%q", post.Code, post.Header().Get("Allow"))
	}
}

func TestReplayHandlerConcurrentClientsReadConsistentArchive(t *testing.T) {
	fixture := newArchiveFixture(t, true)
	handler, err := NewReplayHandler(fixture.archive)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	paths := map[string][]byte{
		"/observer-mode/rest/consumer/version":                               []byte("capture-fixture-1.2.3"),
		"/observer-mode/rest/consumer/getGameDataChunk/KR/123456789/1/token": chunkData[1],
		"/observer-mode/rest/consumer/getKeyFrame/KR/123456789/2/token":      keyFrameData[2],
		"/observer-mode/rest/consumer/endOfGameStats/KR/123456789/null":      testStats,
		"/observer-mode/rest/consumer/getLastChunkInfo/KR/123456789/0/token": nil,
		"/observer-mode/rest/consumer/getGameMetaData/KR/123456789/0/token":  nil,
	}
	const clients = 12
	var wg sync.WaitGroup
	errCh := make(chan error, clients*len(paths))
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path, want := range paths {
				response, err := http.Get(server.URL + path)
				if err != nil {
					errCh <- err
					continue
				}
				body, readErr := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if readErr != nil {
					errCh <- readErr
					continue
				}
				if response.StatusCode != http.StatusOK {
					errCh <- fmt.Errorf("GET %s returned %d", path, response.StatusCode)
					continue
				}
				if want != nil && !bytes.Equal(body, want) {
					errCh <- fmt.Errorf("GET %s returned inconsistent bytes", path)
				}
				if want == nil && len(body) == 0 {
					errCh <- fmt.Errorf("GET %s returned empty JSON", path)
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func request(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func metadataWithAvailability(t *testing.T, field, entries string) []byte {
	t.Helper()
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(testMetadata, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata[field] = json.RawMessage(entries)
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func copyArchiveWithComplete(fixture archiveFixture, destination string, complete bool) error {
	if err := filepath.WalkDir(fixture.dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(fixture.dir, path)
		if err != nil {
			return err
		}
		to := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(to, 0o700)
		}
		if rel == "manifest.json" {
			manifest := fixture.archive.Manifest()
			manifest.Complete = complete
			data, err := json.Marshal(manifest)
			if err != nil {
				return err
			}
			return os.WriteFile(to, data, 0o600)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(to, data, 0o600)
	}); err != nil {
		return err
	}
	return nil
}
