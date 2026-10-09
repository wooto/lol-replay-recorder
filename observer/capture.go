package observer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Capture downloads observer chunks and keyframes into a new archive directory.
// The returned archive is retained on failure so callers can inspect partial
// work. A 404 for an advertised file is retried on later polls while
// the game is live. The archive is marked complete only after the server has
// announced an end and every expected payload has been saved as a readable,
// bounded file.
func (c *Client) Capture(ctx context.Context, game Game, destination string) (*Archive, error) {
	if c == nil {
		return nil, errors.New("nil observer client")
	}
	if ctx == nil {
		return nil, errors.New("context cannot be nil")
	}
	if err := validateGame(game); err != nil {
		return nil, err
	}
	if strings.TrimSpace(destination) == "" {
		return nil, errors.New("archive destination is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	archive, err := createArchive(destination, game, c.maxResponseBytes)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Archive, error) { return archive, err }

	version, err := c.fetchVersion(ctx)
	if err != nil {
		return fail(err)
	}
	if err := archive.replaceVersion(version); err != nil {
		return fail(err)
	}
	var previous ChunkInfo
	for {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}

		metadataRaw, metadata, err := c.fetchMetadata(ctx, game)
		if err != nil {
			return fail(err)
		}
		if err := archive.replaceMetadata(metadataRaw); err != nil {
			return fail(err)
		}

		infoRaw, info, err := c.fetchLastChunkInfo(ctx, game)
		if err != nil {
			return fail(err)
		}
		if info.ChunkID < previous.ChunkID || info.KeyFrameID < previous.KeyFrameID {
			return fail(errors.New("observer last chunk info moved backwards"))
		}
		previous = info
		if err := archive.replaceLastChunkInfo(info, infoRaw); err != nil {
			return fail(err)
		}
		current := archive.Manifest()
		current.LastChunk = info
		if err := archive.commitManifest(current); err != nil {
			return fail(err)
		}

		maxChunkID := maxID(info.ChunkID, metadata.LastChunkID)
		maxKeyFrameID := maxID(info.KeyFrameID, metadata.LastKeyFrameID)
		ended, finalChunkID, finalKeyFrameID, endErr := resolveEndTarget(metadata, info)
		if endErr != nil {
			return fail(fmt.Errorf("%w: inconsistent server end markers: %v", ErrArchiveIncomplete, endErr))
		}
		if ended {
			maxChunkID, maxKeyFrameID = finalChunkID, finalKeyFrameID
		}
		if maxChunkID > maxArchiveIDs || maxKeyFrameID > maxArchiveIDs {
			return fail(fmt.Errorf("observer ID exceeds the %d-item archive limit", maxArchiveIDs))
		}

		missingChunks, err := c.downloadRange(ctx, archive, game, "chunks", maxChunkID)
		if err != nil {
			return fail(err)
		}
		missingKeyFrames, err := c.downloadRange(ctx, archive, game, "keyframes", maxKeyFrameID)
		if err != nil {
			return fail(err)
		}

		if ended {
			if len(missingChunks) != 0 || len(missingKeyFrames) != 0 ||
				!hasExactIDs(archive.Manifest().ChunkIDs, finalChunkID) ||
				!hasExactIDs(archive.Manifest().KeyFrameIDs, finalKeyFrameID) {
				return fail(incompleteArchiveError(missingChunks, missingKeyFrames, finalChunkID, finalKeyFrameID))
			}
			if _, err := archive.validateListedFiles(); err != nil {
				return fail(fmt.Errorf("%w: %v", ErrArchiveIncomplete, err))
			}
			normalizedInfo := info
			normalizedInfo.EndGameChunkID = finalChunkID
			normalizedInfo.KeyFrameID = finalKeyFrameID
			lastInfoJSON, err := json.Marshal(normalizedInfo)
			if err != nil {
				return fail(err)
			}
			if err := archive.replaceLastChunkInfo(normalizedInfo, lastInfoJSON); err != nil {
				return fail(err)
			}
			stats, err := c.fetchEndStats(ctx, game)
			if err == nil {
				if err := archive.writeEndStats(stats); err != nil {
					return fail(err)
				}
			} else if !isNotFound(err) {
				return fail(err)
			}
			if err := archive.markComplete(finalChunkID, finalKeyFrameID, info); err != nil {
				return fail(fmt.Errorf("%w: %v", ErrArchiveIncomplete, err))
			}
			return archive, nil
		}

		if err := waitForPoll(ctx, c.pollInterval); err != nil {
			return fail(err)
		}
	}
}

func createArchive(destination string, game Game, maxResponseBytes int64) (*Archive, error) {
	path, err := filepath.Abs(destination)
	if err != nil {
		return nil, fmt.Errorf("resolve archive destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create archive parent directory: %w", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, ErrOutputExists
		}
		return nil, fmt.Errorf("create archive directory: %w", err)
	}
	a := &Archive{
		path: path,
		manifest: Manifest{
			FormatVersion: archiveFormatVersion,
			Game:          game,
			ChunkIDs:      []uint64{},
			KeyFrameIDs:   []uint64{},
		},
		metadata:         json.RawMessage(`{}`),
		maxResponseBytes: maxResponseBytes,
	}
	if err := os.Mkdir(filepath.Join(path, "chunks"), 0o700); err != nil {
		return a, fmt.Errorf("create chunks directory: %w", err)
	}
	if err := os.Mkdir(filepath.Join(path, "keyframes"), 0o700); err != nil {
		return a, fmt.Errorf("create keyframes directory: %w", err)
	}
	for _, initial := range []struct {
		name string
		data []byte
		mode os.FileMode
	}{
		{name: "metadata.json", data: []byte(`{}`), mode: 0o600},
		{name: "version.txt", data: nil, mode: 0o600},
		{name: "last-chunk-info.json", data: []byte(`{}`), mode: 0o600},
	} {
		if err := writeArchiveFile(path, initial.name, initial.data, initial.mode); err != nil {
			return a, fmt.Errorf("initialize archive %s: %w", initial.name, err)
		}
	}
	if err := a.commitManifest(a.manifest); err != nil {
		return a, fmt.Errorf("initialize archive manifest: %w", err)
	}
	return a, nil
}

func (c *Client) downloadRange(ctx context.Context, archive *Archive, game Game, kind string, target uint64) ([]uint64, error) {
	manifest := archive.Manifest()
	ids := manifest.ChunkIDs
	if kind == "keyframes" {
		ids = manifest.KeyFrameIDs
	}
	missing := make([]uint64, 0)
	for id := uint64(1); id <= target; id++ {
		if containsID(ids, id) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return missing, err
		}
		payload, err := c.fetchPayload(ctx, game, kind, id)
		if isNotFound(err) {
			missing = append(missing, id)
			continue
		}
		if err != nil {
			return missing, fmt.Errorf("download %s %d: %w", kind, id, err)
		}
		if err := archive.storePayload(kind, id, payload); err != nil {
			return missing, fmt.Errorf("save %s %d: %w", kind, id, err)
		}
		ids = append(ids, id)
	}
	return missing, nil
}

func incompleteArchiveError(chunks, keyFrames []uint64, finalChunkID, finalKeyFrameID uint64) error {
	if len(chunks) != 0 {
		return fmt.Errorf("%w: chunk %d of %d is unavailable after the end marker", ErrArchiveIncomplete, chunks[0], finalChunkID)
	}
	if len(keyFrames) != 0 {
		return fmt.Errorf("%w: keyframe %d of %d is unavailable after the end marker", ErrArchiveIncomplete, keyFrames[0], finalKeyFrameID)
	}
	return fmt.Errorf("%w: expected payload ranges are incomplete", ErrArchiveIncomplete)
}

func maxID(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func resolveEndTarget(metadata observerMetadata, info ChunkInfo) (bool, uint64, uint64, error) {
	ended := metadata.GameEnded || metadata.EndGameChunkID != 0 || info.EndGameChunkID != 0
	if !ended {
		return false, 0, 0, nil
	}
	finalChunkID, err := agreePositiveIDs("final chunk", metadata.EndGameChunkID, info.EndGameChunkID)
	if err != nil {
		return true, 0, 0, err
	}
	if finalChunkID == 0 {
		finalChunkID = info.ChunkID
		if finalChunkID == 0 {
			finalChunkID = metadata.LastChunkID
		}
	}
	if finalChunkID == 0 {
		return true, 0, 0, errors.New("server end marker has no final chunk ID")
	}
	if info.ChunkID != finalChunkID {
		return true, 0, 0, fmt.Errorf("last chunk snapshot ID %d does not match final chunk ID %d", info.ChunkID, finalChunkID)
	}
	if metadata.LastChunkID > finalChunkID {
		return true, 0, 0, fmt.Errorf("metadata lastChunkId %d exceeds final chunk ID %d", metadata.LastChunkID, finalChunkID)
	}
	finalKeyFrameID, err := agreePositiveIDs("final keyframe", metadata.EndGameKeyFrameID, info.KeyFrameID)
	if err != nil {
		return true, 0, 0, err
	}
	if finalKeyFrameID == 0 {
		finalKeyFrameID = maxID(metadata.LastKeyFrameID, info.KeyFrameID)
	}
	if metadata.LastKeyFrameID > finalKeyFrameID {
		return true, 0, 0, fmt.Errorf("metadata lastKeyFrameId %d exceeds final keyframe ID %d", metadata.LastKeyFrameID, finalKeyFrameID)
	}
	return true, finalChunkID, finalKeyFrameID, nil
}

func agreePositiveIDs(name string, ids ...uint64) (uint64, error) {
	var result uint64
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if result != 0 && result != id {
			return 0, fmt.Errorf("conflicting %s IDs %d and %d", name, result, id)
		}
		result = id
	}
	return result, nil
}

func waitForPoll(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (a *Archive) writeEndStats(data []byte) error {
	if len(data) == 0 || int64(len(data)) > minResponseLimit(a.maxResponseBytes, maxEndStatsBytes) || !json.Valid(data) {
		return errors.New("invalid end-of-game statistics payload")
	}
	return writeArchiveFile(a.path, "end-of-game-stats.json", data, 0o600)
}
