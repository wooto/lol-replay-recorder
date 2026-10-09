// Package observer serves a captured replay through the legacy League observer
// HTTP routes. It is an experimental, offline protocol adapter; compatibility
// with current League clients has not been verified.
package observer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const consumerPrefix = "/observer-mode/rest/consumer/"

// NewReplayHandler returns an HTTP handler for a complete, validated archive.
// The handler has no per-client progress state: every last-chunk request
// returns the archived final snapshot. The caller chooses and binds the
// listener; this function never starts a server.
func NewReplayHandler(archive *Archive) (http.Handler, error) {
	if archive == nil {
		return nil, errors.New("observer: archive is nil")
	}
	manifest := archive.Manifest()
	if !manifest.Complete {
		return nil, errors.New("observer: archive is incomplete")
	}
	if !replayValidPlatformID(manifest.Game.PlatformID) || manifest.Game.GameID == 0 {
		return nil, errors.New("observer: archive game identity is invalid")
	}
	if len(manifest.ChunkIDs) == 0 {
		return nil, errors.New("observer: complete archive has no chunks")
	}
	if !replayIDsContiguous(manifest.ChunkIDs) || !replayIDsContiguous(manifest.KeyFrameIDs) {
		return nil, errors.New("observer: complete archive has gaps in its chunk or keyframe IDs")
	}

	last := archive.LastChunkInfo()
	if last.ChunkID == 0 {
		last = manifest.LastChunk
	}
	finalChunkID := manifest.ChunkIDs[len(manifest.ChunkIDs)-1]
	if last.ChunkID == 0 || last.ChunkID != finalChunkID {
		return nil, errors.New("observer: archive final chunk snapshot is invalid")
	}
	if last.EndGameChunkID == 0 {
		last.EndGameChunkID = manifest.LastChunk.EndGameChunkID
	}
	if last.EndGameChunkID != finalChunkID {
		return nil, errors.New("observer: archive announced end-game chunk does not match its final chunk")
	}
	if len(manifest.KeyFrameIDs) == 0 {
		if last.KeyFrameID != 0 {
			return nil, errors.New("observer: archive final chunk snapshot announces a missing keyframe")
		}
	} else if last.KeyFrameID != manifest.KeyFrameIDs[len(manifest.KeyFrameIDs)-1] {
		return nil, errors.New("observer: archive final chunk snapshot does not reach its final keyframe")
	}
	last.NextAvailableChunk = 0

	metadata, err := replayNormalizeMetadata(archive.Metadata(), manifest, last)
	if err != nil {
		return nil, fmt.Errorf("observer: invalid archive metadata: %w", err)
	}
	version := strings.TrimSpace(archive.Version())
	if version == "" {
		return nil, errors.New("observer: archive version is empty")
	}
	lastJSON, err := json.Marshal(last)
	if err != nil {
		return nil, fmt.Errorf("observer: encode final chunk snapshot: %w", err)
	}

	return &replayHandler{
		archive:  archive,
		game:     manifest.Game,
		version:  []byte(version),
		metadata: metadata,
		lastInfo: lastJSON,
	}, nil
}

type replayHandler struct {
	archive  *Archive
	game     Game
	version  []byte
	metadata []byte
	lastInfo []byte
}

func (h *replayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL == nil {
		http.NotFound(w, r)
		return
	}

	path := r.URL.Path
	if !strings.HasPrefix(path, consumerPrefix) {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, consumerPrefix), "/")
	if len(parts) == 1 && parts[0] == "version" {
		replayWritePayload(w, r, "text/plain; charset=utf-8", h.version)
		return
	}
	var payload []byte
	contentType := "application/json"
	var err error
	switch parts[0] {
	case "getGameMetaData":
		if len(parts) != 5 || !h.matchesGame(parts[1], parts[2]) || !replayIsFlag(parts[3]) || parts[4] != "token" {
			http.NotFound(w, r)
			return
		}
		payload = h.metadata
	case "getLastChunkInfo":
		if len(parts) != 5 || !h.matchesGame(parts[1], parts[2]) || !replayIsFlag(parts[3]) || parts[4] != "token" {
			http.NotFound(w, r)
			return
		}
		payload = h.lastInfo
	case "getGameDataChunk":
		if len(parts) != 5 || !h.matchesGame(parts[1], parts[2]) || parts[4] != "token" {
			http.NotFound(w, r)
			return
		}
		id, ok := replayParseID(parts[3])
		if !ok {
			http.NotFound(w, r)
			return
		}
		payload, err = h.archive.ReadChunk(id)
		contentType = "application/octet-stream"
	case "getKeyFrame":
		if len(parts) != 5 || !h.matchesGame(parts[1], parts[2]) || parts[4] != "token" {
			http.NotFound(w, r)
			return
		}
		id, ok := replayParseID(parts[3])
		if !ok {
			http.NotFound(w, r)
			return
		}
		payload, err = h.archive.ReadKeyFrame(id)
		contentType = "application/octet-stream"
	case "endOfGameStats":
		if len(parts) != 4 || !h.matchesGame(parts[1], parts[2]) || parts[3] != "null" {
			http.NotFound(w, r)
			return
		}
		payload, err = h.archive.ReadEndStats()
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		// Archive read errors can include local paths. Never expose them to a
		// remote caller; missing optional stats and missing IDs are ordinary 404s.
		http.NotFound(w, r)
		return
	}
	replayWritePayload(w, r, contentType, payload)
}

func (h *replayHandler) matchesGame(platform, rawID string) bool {
	id, ok := replayParseID(rawID)
	return ok && platform == h.game.PlatformID && id == h.game.GameID
}

func replayWritePayload(w http.ResponseWriter, r *http.Request, contentType string, payload []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(payload)
}

func replayParseID(value string) (uint64, bool) {
	if value == "" {
		return 0, false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	id, err := strconv.ParseUint(value, 10, 64)
	return id, err == nil && id != 0
}

func replayIsFlag(value string) bool { return value == "0" || value == "1" }

func replayValidPlatformID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func replayIDsContiguous(ids []uint64) bool {
	for i, id := range ids {
		if id != uint64(i+1) {
			return false
		}
	}
	return true
}

// normalizeMetadata preserves source metadata fields and values where known,
// while making the availability fields agree with the complete archive. A
// keyframe's nextChunkId is retained only when present in source metadata; the
// handler does not infer that relationship from numeric IDs.
func replayNormalizeMetadata(raw json.RawMessage, manifest Manifest, last ChunkInfo) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var metadata map[string]any
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("decode JSON object: %w", err)
	}
	if metadata == nil {
		return nil, errors.New("expected JSON object")
	}
	if err := replayRequireEOF(decoder); err != nil {
		return nil, err
	}

	gameKey, err := replayMetadataGameKey(metadata["gameKey"], manifest.Game)
	if err != nil {
		return nil, err
	}
	metadata["gameKey"] = gameKey
	metadata["gameEnded"] = true
	metadata["lastChunkId"] = manifest.ChunkIDs[len(manifest.ChunkIDs)-1]
	var finalKeyFrameID uint64
	if len(manifest.KeyFrameIDs) > 0 {
		finalKeyFrameID = manifest.KeyFrameIDs[len(manifest.KeyFrameIDs)-1]
	}
	metadata["lastKeyFrameId"] = finalKeyFrameID
	metadata["endGameChunkId"] = last.EndGameChunkID
	metadata["endGameKeyFrameId"] = finalKeyFrameID

	chunkDuration := json.Number("0")
	if duration, ok := metadata["chunkTimeInterval"].(json.Number); ok {
		if n, e := duration.Int64(); e == nil && n > 0 {
			chunkDuration = duration
		}
	}
	metadata["pendingAvailableChunkInfo"], err = replayNormalizeAvailabilityArray(metadata["pendingAvailableChunkInfo"], manifest.ChunkIDs, "chunkId", "duration", chunkDuration)
	if err != nil {
		return nil, fmt.Errorf("pendingAvailableChunkInfo: %w", err)
	}
	metadata["pendingAvailableKeyFrameInfo"], err = replayNormalizeAvailabilityArray(metadata["pendingAvailableKeyFrameInfo"], manifest.KeyFrameIDs, "keyFrameId", "", "")
	if err != nil {
		return nil, fmt.Errorf("pendingAvailableKeyFrameInfo: %w", err)
	}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func replayMetadataGameKey(value any, game Game) (map[string]any, error) {
	key := make(map[string]any)
	if value != nil {
		var ok bool
		key, ok = value.(map[string]any)
		if !ok {
			return nil, errors.New("gameKey must be an object")
		}
		if value, exists := key["platformId"]; exists {
			existing, ok := value.(string)
			if !ok || existing != "" && !strings.EqualFold(existing, game.PlatformID) {
				return nil, errors.New("gameKey platformId does not match archive")
			}
		}
		if value, exists := key["gameId"]; exists {
			existing, ok := value.(json.Number)
			if !ok {
				return nil, errors.New("gameKey gameId does not match archive")
			}
			id, err := strconv.ParseUint(string(existing), 10, 64)
			if err != nil || id != game.GameID {
				return nil, errors.New("gameKey gameId does not match archive")
			}
		}
	}
	key["platformId"] = game.PlatformID
	key["gameId"] = game.GameID
	return key, nil
}

func replayNormalizeAvailabilityArray(value any, ids []uint64, idKey, durationKey string, duration any) ([]map[string]any, error) {
	byID := make(map[uint64]map[string]any)
	if value != nil {
		items, ok := value.([]any)
		if !ok {
			return nil, errors.New("must be an array")
		}
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("entries must be objects")
			}
			id, ok := replayJSONUint(entry[idKey])
			if !ok || id == 0 || id > maxArchiveIDs {
				return nil, fmt.Errorf("entry has an invalid %s", idKey)
			}
			if _, exists := byID[id]; exists {
				return nil, fmt.Errorf("duplicate %s %d", idKey, id)
			}
			byID[id] = entry
		}
	}

	result := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		entry, exists := byID[id]
		if !exists {
			entry = map[string]any{idKey: id}
			if durationKey != "" && duration != nil && duration != json.Number("0") {
				entry[durationKey] = duration
			}
		}
		entry[idKey] = id
		result = append(result, entry)
	}
	return result, nil
}

func replayJSONUint(value any) (uint64, bool) {
	n, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseUint(string(n), 10, 64)
	return id, err == nil
}

func replayRequireEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return fmt.Errorf("trailing data: %w", err)
	}
	return nil
}
