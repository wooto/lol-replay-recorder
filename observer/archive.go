package observer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	archiveFormatVersion = 1
	maxArchiveIDs        = 100_000
	maxArchiveFileBytes  = int64(256 << 20)
	maxMetadataBytes     = int64(16 << 20)
	maxEndStatsBytes     = int64(16 << 20)
	maxManifestBytes     = int64(16 << 20)
	maxVersionBytes      = int64(4 << 10)
	maxChunkInfoBytes    = int64(64 << 10)
	maxArchiveBytes      = int64(8 << 30)
)

// Archive provides read access to a persisted observer capture.
type Archive struct {
	mu               sync.RWMutex
	path             string
	manifest         Manifest
	metadata         json.RawMessage
	version          string
	maxResponseBytes int64
	storedBytes      int64
}

type observerMetadata struct {
	GameKey struct {
		GameID     uint64 `json:"gameId"`
		PlatformID string `json:"platformId"`
	} `json:"gameKey"`
	GameEnded         bool   `json:"gameEnded"`
	LastChunkID       uint64 `json:"lastChunkId"`
	LastKeyFrameID    uint64 `json:"lastKeyFrameId"`
	EndGameChunkID    uint64 `json:"endGameChunkId"`
	EndGameKeyFrameID uint64 `json:"endGameKeyFrameId"`
}

// OpenArchive opens and validates a saved observer archive. It rejects symlinked
// archive entries, malformed manifests, path traversal, and files above the
// package's hard read limits.
func OpenArchive(path string) (*Archive, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("archive path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve archive path: %w", err)
	}
	if err := requireDirectory(abs); err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}

	manifestData, err := readArchiveFile(abs, "manifest.json", maxManifestBytes, false)
	if err != nil {
		return nil, fmt.Errorf("read archive manifest: %w", err)
	}
	var manifest Manifest
	if err := decodeSingleJSON(manifestData, &manifest, true); err != nil {
		return nil, fmt.Errorf("decode archive manifest: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return nil, fmt.Errorf("invalid archive manifest: %w", err)
	}
	if err := requireDirectory(filepath.Join(abs, "chunks")); err != nil {
		return nil, fmt.Errorf("invalid chunks directory: %w", err)
	}
	if err := requireDirectory(filepath.Join(abs, "keyframes")); err != nil {
		return nil, fmt.Errorf("invalid keyframes directory: %w", err)
	}

	metadata, err := readArchiveFile(abs, "metadata.json", maxMetadataBytes, false)
	if err != nil {
		return nil, fmt.Errorf("read archive metadata: %w", err)
	}
	metadataInfo, err := parseMetadata(metadata, manifest.Game, false)
	if err != nil {
		return nil, fmt.Errorf("invalid archive metadata: %w", err)
	}
	versionData, err := readArchiveFile(abs, "version.txt", maxVersionBytes, true)
	if err != nil {
		return nil, fmt.Errorf("read archive version: %w", err)
	}
	version := strings.TrimSpace(string(versionData))
	infoData, err := readArchiveFile(abs, "last-chunk-info.json", maxChunkInfoBytes, false)
	if err != nil {
		return nil, fmt.Errorf("read archive chunk info: %w", err)
	}
	var diskInfo ChunkInfo
	if err := decodeSingleJSON(infoData, &diskInfo, false); err != nil {
		return nil, fmt.Errorf("decode archive chunk info: %w", err)
	}
	if err := validateChunkInfo(diskInfo); err != nil {
		return nil, fmt.Errorf("invalid archive chunk info: %w", err)
	}
	if manifest.Complete {
		if diskInfo != manifest.LastChunk {
			return nil, errors.New("complete archive chunk info does not match its manifest")
		}
		if err := validateCompleteMetadata(metadataInfo, manifest); err != nil {
			return nil, fmt.Errorf("complete archive metadata disagrees with its manifest: %w", err)
		}
	}

	a := &Archive{
		path:             abs,
		manifest:         cloneManifest(manifest),
		metadata:         append(json.RawMessage(nil), metadata...),
		version:          version,
		maxResponseBytes: maxArchiveFileBytes,
	}
	storedBytes, err := a.validateListedFiles()
	if err != nil {
		return nil, err
	}
	a.storedBytes = storedBytes
	return a, nil
}

// Manifest returns a copy of the archive manifest. Its ID slices are copied so
// callers cannot mutate the archive's in-memory state.
func (a *Archive) Manifest() Manifest {
	if a == nil {
		return Manifest{}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return cloneManifest(a.manifest)
}

// Metadata returns the raw metadata JSON from the observer server.
func (a *Archive) Metadata() json.RawMessage {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append(json.RawMessage(nil), a.metadata...)
}

// Version returns the observer server version recorded with the archive.
func (a *Archive) Version() string {
	if a == nil {
		return ""
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.version
}

// LastChunkInfo returns the latest observer chunk snapshot stored in the
// manifest. It is a value type and can be used directly by callers.
func (a *Archive) LastChunkInfo() ChunkInfo {
	if a == nil {
		return ChunkInfo{}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.manifest.LastChunk
}

// ReadChunk returns a copy of a listed chunk payload.
func (a *Archive) ReadChunk(id uint64) ([]byte, error) {
	if a == nil {
		return nil, errors.New("nil archive")
	}
	a.mu.RLock()
	listed := containsID(a.manifest.ChunkIDs, id)
	root, limit := a.path, a.maxResponseBytes
	a.mu.RUnlock()
	if !listed {
		return nil, os.ErrNotExist
	}
	return readArchiveFile(root, filepath.Join("chunks", idFileName(id)), limit, false)
}

// ReadKeyFrame returns a copy of a listed keyframe payload.
func (a *Archive) ReadKeyFrame(id uint64) ([]byte, error) {
	if a == nil {
		return nil, errors.New("nil archive")
	}
	a.mu.RLock()
	listed := containsID(a.manifest.KeyFrameIDs, id)
	root, limit := a.path, a.maxResponseBytes
	a.mu.RUnlock()
	if !listed {
		return nil, os.ErrNotExist
	}
	return readArchiveFile(root, filepath.Join("keyframes", idFileName(id)), limit, false)
}

// ReadEndStats returns the optional raw end-of-game statistics JSON. Servers
// may not expose this endpoint, in which case os.ErrNotExist is returned.
func (a *Archive) ReadEndStats() ([]byte, error) {
	if a == nil {
		return nil, errors.New("nil archive")
	}
	a.mu.RLock()
	root, limit := a.path, minResponseLimit(a.maxResponseBytes, maxEndStatsBytes)
	a.mu.RUnlock()
	return readArchiveFile(root, "end-of-game-stats.json", limit, false)
}

func cloneManifest(m Manifest) Manifest {
	m.ChunkIDs = append([]uint64(nil), m.ChunkIDs...)
	m.KeyFrameIDs = append([]uint64(nil), m.KeyFrameIDs...)
	return m
}

func containsID(ids []uint64, id uint64) bool {
	i := sort.Search(len(ids), func(i int) bool { return ids[i] >= id })
	return i < len(ids) && ids[i] == id
}

func idFileName(id uint64) string { return fmt.Sprintf("%d.bin", id) }

func validateGame(game Game) error {
	if game.GameID == 0 {
		return errors.New("game ID must be positive")
	}
	if game.PlatformID == "" || len(game.PlatformID) > 32 {
		return errors.New("platform ID is required and must be at most 32 characters")
	}
	for _, r := range game.PlatformID {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return errors.New("platform ID contains unsupported characters")
		}
	}
	return nil
}

func validateManifest(m Manifest) error {
	if m.FormatVersion != archiveFormatVersion {
		return fmt.Errorf("unsupported format version %d", m.FormatVersion)
	}
	if err := validateGame(m.Game); err != nil {
		return err
	}
	if len(m.ChunkIDs) > maxArchiveIDs || len(m.KeyFrameIDs) > maxArchiveIDs {
		return errors.New("archive ID count exceeds limit")
	}
	if err := validateIDList(m.ChunkIDs); err != nil {
		return fmt.Errorf("invalid chunk IDs: %w", err)
	}
	if err := validateIDList(m.KeyFrameIDs); err != nil {
		return fmt.Errorf("invalid keyframe IDs: %w", err)
	}
	if err := validateChunkInfo(m.LastChunk); err != nil {
		return err
	}
	if m.Complete {
		if m.LastChunk.EndGameChunkID == 0 || m.LastChunk.EndGameChunkID > uint64(len(m.ChunkIDs)) || uint64(len(m.ChunkIDs)) != m.LastChunk.EndGameChunkID || m.LastChunk.ChunkID != m.LastChunk.EndGameChunkID {
			return errors.New("complete archive has no contiguous final chunk range")
		}
		if m.LastChunk.KeyFrameID != uint64(len(m.KeyFrameIDs)) {
			return errors.New("complete archive keyframes do not reach the announced final ID")
		}
		for i, id := range m.ChunkIDs {
			if id != uint64(i+1) {
				return errors.New("complete archive has a chunk gap")
			}
		}
		for i, id := range m.KeyFrameIDs {
			if id != uint64(i+1) {
				return errors.New("complete archive has a keyframe gap")
			}
		}
	}
	return nil
}

func validateIDList(ids []uint64) error {
	var prior uint64
	for _, id := range ids {
		if id == 0 || id > maxArchiveIDs || id <= prior {
			return errors.New("IDs must be positive, bounded, unique, and increasing")
		}
		prior = id
	}
	return nil
}

func validateChunkInfo(info ChunkInfo) error {
	for _, id := range []uint64{info.ChunkID, info.KeyFrameID, info.NextChunkID, info.EndStartupChunkID, info.StartGameChunkID, info.EndGameChunkID} {
		if id > maxArchiveIDs {
			return errors.New("chunk info ID exceeds limit")
		}
	}
	if info.Duration < 0 || info.AvailableSince < 0 || info.NextAvailableChunk < 0 {
		return errors.New("chunk info contains a negative duration or availability value")
	}
	return nil
}

func validateCompleteMetadata(metadata observerMetadata, manifest Manifest) error {
	finalChunkID := manifest.LastChunk.EndGameChunkID
	finalKeyFrameID := manifest.LastChunk.KeyFrameID
	if metadata.EndGameChunkID != 0 && metadata.EndGameChunkID != finalChunkID {
		return errors.New("metadata endGameChunkId does not match the manifest")
	}
	if metadata.LastChunkID > finalChunkID {
		return errors.New("metadata lastChunkId exceeds the manifest final chunk")
	}
	if metadata.EndGameKeyFrameID != 0 && metadata.EndGameKeyFrameID != finalKeyFrameID {
		return errors.New("metadata endGameKeyFrameId does not match the manifest")
	}
	if metadata.LastKeyFrameID > finalKeyFrameID {
		return errors.New("metadata lastKeyFrameId exceeds the manifest final keyframe")
	}
	return nil
}

func parseMetadata(data []byte, expected Game, requireGameKey bool) (observerMetadata, error) {
	var root map[string]json.RawMessage
	if err := decodeSingleJSON(data, &root, false); err != nil {
		return observerMetadata{}, err
	}
	if root == nil {
		return observerMetadata{}, errors.New("metadata must be a JSON object")
	}
	if raw, present := root["gameEnded"]; present && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return observerMetadata{}, errors.New("metadata gameEnded cannot be null")
	}
	for _, key := range []string{"lastChunkId", "lastKeyFrameId", "endGameChunkId", "endGameKeyFrameId"} {
		raw, present := root[key]
		if !present {
			continue
		}
		trimmed := bytes.TrimSpace(raw)
		if bytes.Equal(trimmed, []byte("null")) {
			return observerMetadata{}, fmt.Errorf("metadata field %s cannot be null", key)
		}
		var id uint64
		if err := json.Unmarshal(raw, &id); err != nil {
			return observerMetadata{}, fmt.Errorf("metadata field %s must be an unsigned integer: %w", key, err)
		}
		if id > maxArchiveIDs {
			return observerMetadata{}, fmt.Errorf("metadata field %s exceeds the ID limit", key)
		}
	}
	var metadata observerMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return observerMetadata{}, err
	}
	if requireGameKey || len(root["gameKey"]) != 0 {
		if metadata.GameKey.GameID == 0 || metadata.GameKey.PlatformID == "" {
			return observerMetadata{}, errors.New("metadata gameKey is incomplete")
		}
		if metadata.GameKey.GameID != expected.GameID || !strings.EqualFold(metadata.GameKey.PlatformID, expected.PlatformID) {
			return observerMetadata{}, errors.New("metadata gameKey does not match requested game")
		}
	}
	for _, id := range []uint64{metadata.LastChunkID, metadata.LastKeyFrameID, metadata.EndGameChunkID, metadata.EndGameKeyFrameID} {
		if id > maxArchiveIDs {
			return observerMetadata{}, errors.New("metadata ID exceeds limit")
		}
	}
	return metadata, nil
}

func decodeSingleJSON(data []byte, out any, disallowUnknown bool) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if disallowUnknown {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return errors.New("JSON contains trailing data")
		}
		return fmt.Errorf("JSON has invalid trailing data: %w", err)
	}
	return nil
}

func (a *Archive) validateListedFiles() (int64, error) {
	var total int64
	for _, item := range []struct {
		kind string
		ids  []uint64
	}{
		{kind: "chunks", ids: a.manifest.ChunkIDs},
		{kind: "keyframes", ids: a.manifest.KeyFrameIDs},
	} {
		for _, id := range item.ids {
			path := filepath.Join(item.kind, idFileName(id))
			size, err := validateArchiveFile(a.path, path, maxArchiveFileBytes, false)
			if err != nil {
				return 0, fmt.Errorf("validate %s file %d: %w", item.kind, id, err)
			}
			total += size
			if total > maxArchiveBytes {
				return 0, errors.New("archive payload exceeds total size limit")
			}
		}
	}
	return total, nil
}

func requireDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path is not a regular directory")
	}
	return nil
}

// openArchiveFile centralizes the containment, type, size, and identity checks
// used by both payload readers and whole-archive validation.
func openArchiveFile(root, relative string, limit int64) (*os.File, error) {
	path := filepath.Join(root, relative)
	if !withinRoot(root, path) {
		return nil, errors.New("archive path escapes root")
	}
	if err := checkParentDirectories(root, filepath.Dir(path)); err != nil {
		return nil, err
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("archive entry is not a regular file")
	}
	if before.Size() < 0 || before.Size() > limit {
		return nil, errors.New("archive file exceeds size limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = f.Close()
		return nil, errors.New("archive file changed while opening")
	}
	return f, nil
}

func readArchiveFile(root, relative string, limit int64, allowEmpty bool) ([]byte, error) {
	f, err := openArchiveFile(root, relative, limit)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("archive file exceeds size limit")
	}
	if !allowEmpty && len(data) == 0 {
		return nil, errors.New("archive file is empty")
	}
	return data, nil
}

func validateArchiveFile(root, relative string, limit int64, allowEmpty bool) (int64, error) {
	f, err := openArchiveFile(root, relative, limit)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	read, err := io.Copy(io.Discard, io.LimitReader(f, limit+1))
	if err != nil {
		return 0, err
	}
	if read > limit {
		return 0, errors.New("archive file exceeds size limit")
	}
	if !allowEmpty && read == 0 {
		return 0, errors.New("archive file is empty")
	}
	return read, nil
}

func withinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}

func checkParentDirectories(root, parent string) error {
	if err := requireDirectory(root); err != nil {
		return err
	}
	rel, err := filepath.Rel(root, parent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("archive directory escapes root")
	}
	if rel == "." {
		return nil
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		if err := requireDirectory(current); err != nil {
			return err
		}
	}
	return nil
}

func writeArchiveFile(root, relative string, data []byte, mode os.FileMode) error {
	path := filepath.Join(root, relative)
	if !withinRoot(root, path) {
		return errors.New("archive path escapes root")
	}
	if err := checkParentDirectories(root, filepath.Dir(path)); err != nil {
		return err
	}
	return writeAtomicFile(path, data, mode)
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".observer-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func (a *Archive) replaceMetadata(data []byte) error {
	if int64(len(data)) > minResponseLimit(a.maxResponseBytes, maxMetadataBytes) {
		return errors.New("observer metadata exceeds size limit")
	}
	if _, err := parseMetadata(data, a.manifest.Game, true); err != nil {
		return err
	}
	if err := writeArchiveFile(a.path, "metadata.json", data, 0o600); err != nil {
		return err
	}
	a.mu.Lock()
	a.metadata = append(json.RawMessage(nil), data...)
	a.mu.Unlock()
	return nil
}

func (a *Archive) replaceVersion(version string) error {
	if len(version) == 0 || int64(len(version)) > maxVersionBytes || strings.ContainsAny(version, "\r\n\x00") {
		return errors.New("observer version is empty or invalid")
	}
	if err := writeArchiveFile(a.path, "version.txt", []byte(version), 0o600); err != nil {
		return err
	}
	a.mu.Lock()
	a.version = version
	a.mu.Unlock()
	return nil
}

func (a *Archive) replaceLastChunkInfo(info ChunkInfo, raw []byte) error {
	if err := validateChunkInfo(info); err != nil {
		return err
	}
	if int64(len(raw)) > minResponseLimit(a.maxResponseBytes, maxChunkInfoBytes) {
		return errors.New("observer chunk info exceeds size limit")
	}
	if err := writeArchiveFile(a.path, "last-chunk-info.json", raw, 0o600); err != nil {
		return err
	}
	return nil
}

func (a *Archive) storePayload(kind string, id uint64, data []byte) error {
	if (kind != "chunks" && kind != "keyframes") || id == 0 || id > maxArchiveIDs {
		return errors.New("invalid archive payload identity")
	}
	if len(data) == 0 {
		return errors.New("observer returned an empty payload")
	}
	if int64(len(data)) > a.maxResponseBytes || int64(len(data)) > maxArchiveFileBytes {
		return errors.New("observer payload exceeds size limit")
	}
	a.mu.RLock()
	current := cloneManifest(a.manifest)
	a.mu.RUnlock()
	ids := current.ChunkIDs
	if kind == "keyframes" {
		ids = current.KeyFrameIDs
	}
	if containsID(ids, id) {
		return nil
	}
	if err := a.ensureArchiveBudget(int64(len(data))); err != nil {
		return err
	}
	if err := writeArchiveFile(a.path, filepath.Join(kind, idFileName(id)), data, 0o600); err != nil {
		return err
	}
	ids = insertID(ids, id)
	if kind == "chunks" {
		current.ChunkIDs = ids
	} else {
		current.KeyFrameIDs = ids
	}
	if err := a.commitManifest(current); err != nil {
		return err
	}
	a.mu.Lock()
	a.storedBytes += int64(len(data))
	a.mu.Unlock()
	return nil
}

func insertID(ids []uint64, id uint64) []uint64 {
	index := sort.Search(len(ids), func(i int) bool { return ids[i] >= id })
	if index < len(ids) && ids[index] == id {
		return ids
	}
	ids = append(ids, 0)
	copy(ids[index+1:], ids[index:])
	ids[index] = id
	return ids
}

func (a *Archive) ensureArchiveBudget(next int64) error {
	a.mu.RLock()
	stored := a.storedBytes
	a.mu.RUnlock()
	if next < 0 || next > maxArchiveBytes || stored > maxArchiveBytes-next {
		return errors.New("archive payload exceeds total size limit")
	}
	return nil
}

func (a *Archive) commitManifest(manifest Manifest) error {
	if err := validateManifest(manifest); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if int64(len(data)) > maxManifestBytes {
		return errors.New("archive manifest exceeds size limit")
	}
	if err := writeArchiveFile(a.path, "manifest.json", data, 0o600); err != nil {
		return err
	}
	a.mu.Lock()
	a.manifest = cloneManifest(manifest)
	a.mu.Unlock()
	return nil
}

func (a *Archive) markComplete(finalChunkID, finalKeyFrameID uint64, last ChunkInfo) error {
	if finalChunkID == 0 || finalChunkID > maxArchiveIDs || finalKeyFrameID > maxArchiveIDs || last.ChunkID != finalChunkID {
		return ErrArchiveIncomplete
	}
	current := a.Manifest()
	if !hasExactIDs(current.ChunkIDs, finalChunkID) || !hasExactIDs(current.KeyFrameIDs, finalKeyFrameID) {
		return ErrArchiveIncomplete
	}
	last.EndGameChunkID = finalChunkID
	last.KeyFrameID = finalKeyFrameID
	current.LastChunk = last
	current.Complete = true
	return a.commitManifest(current)
}

func hasExactIDs(ids []uint64, end uint64) bool {
	if uint64(len(ids)) != end {
		return false
	}
	for i, id := range ids {
		if id != uint64(i+1) {
			return false
		}
	}
	return true
}
