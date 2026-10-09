package observer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestOpenArchiveRejectsMalformedAndSymlinkedArchives(t *testing.T) {
	t.Run("path traversal in manifest", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "archive")
		_, err := createArchive(directory, Game{PlatformID: "KR", GameID: 51}, defaultObserverResponseBytes)
		if err != nil {
			t.Fatal(err)
		}
		malformed := []byte(`{"formatVersion":1,"game":{"platformId":"KR","gameId":51},"complete":false,"chunkIds":["../../outside"],"keyFrameIds":[],"lastChunk":{}}`)
		if err := os.WriteFile(filepath.Join(directory, "manifest.json"), malformed, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenArchive(directory); err == nil {
			t.Fatal("OpenArchive accepted a traversal-shaped manifest ID")
		}
	})

	t.Run("complete claim with a chunk gap", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "archive")
		game := Game{PlatformID: "KR", GameID: 52}
		a, err := createArchive(directory, game, defaultObserverResponseBytes)
		if err != nil {
			t.Fatal(err)
		}
		if err := a.replaceVersion("2.0.0"); err != nil {
			t.Fatal(err)
		}
		if err := a.replaceMetadata(testArchiveMetadata(t, game, 3, 1, 3, 1)); err != nil {
			t.Fatal(err)
		}
		if err := a.storePayload("chunks", 1, []byte("one")); err != nil {
			t.Fatal(err)
		}
		if err := a.storePayload("chunks", 3, []byte("three")); err != nil {
			t.Fatal(err)
		}
		if err := a.storePayload("keyframes", 1, []byte("keyframe")); err != nil {
			t.Fatal(err)
		}
		invalid := Manifest{
			FormatVersion: archiveFormatVersion,
			Game:          game,
			Complete:      true,
			ChunkIDs:      []uint64{1, 3},
			KeyFrameIDs:   []uint64{1},
			LastChunk:     ChunkInfo{ChunkID: 3, KeyFrameID: 1, EndGameChunkID: 3},
		}
		encoded, err := json.Marshal(invalid)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "manifest.json"), encoded, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenArchive(directory); err == nil {
			t.Fatal("OpenArchive accepted a complete archive with a chunk gap")
		}
	})

	t.Run("symlinked payload", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "archive")
		archive := createCompleteTestArchive(t, directory, Game{PlatformID: "KR", GameID: 53}, 1, 1)
		_ = archive
		outside := filepath.Join(t.TempDir(), "outside.bin")
		if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		payload := filepath.Join(directory, "chunks", "1.bin")
		if err := os.Remove(payload); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, payload); err != nil {
			t.Skipf("symlinks are unavailable: %v", err)
		}
		if _, err := OpenArchive(directory); err == nil {
			t.Fatal("OpenArchive accepted a symlinked payload")
		}
	})
}

func TestOpenArchiveRejectsNullMetadataFields(t *testing.T) {
	for _, field := range []string{"gameEnded", "lastChunkId", "lastKeyFrameId", "endGameChunkId", "endGameKeyFrameId"} {
		t.Run(field, func(t *testing.T) {
			game := Game{PlatformID: "KR", GameID: 56}
			directory := filepath.Join(t.TempDir(), "archive")
			if _, err := createArchive(directory, game, defaultObserverResponseBytes); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "version.txt"), []byte("2.0.0"), 0o600); err != nil {
				t.Fatal(err)
			}
			metadata := []byte(fmt.Sprintf(`{"gameKey":{"platformId":"KR","gameId":56},"%s":null}`, field))
			if err := os.WriteFile(filepath.Join(directory, "metadata.json"), metadata, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenArchive(directory); err == nil {
				t.Fatalf("OpenArchive accepted null metadata field %s", field)
			}
		})
	}
}

func TestOpenArchiveCompleteAndAccessorsAreDefensive(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "archive")
	game := Game{PlatformID: "KR", GameID: 54}
	createCompleteTestArchive(t, directory, game, 2, 1)
	archive, err := OpenArchive(directory)
	if err != nil {
		t.Fatalf("OpenArchive(): %v", err)
	}
	manifest := archive.Manifest()
	if !manifest.Complete || len(manifest.ChunkIDs) != 2 || manifest.LastChunk.EndGameChunkID != 2 {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	manifest.ChunkIDs[0] = 99
	if got := archive.Manifest().ChunkIDs[0]; got != 1 {
		t.Fatalf("Manifest() exposed mutable IDs: first ID is now %d", got)
	}
	metadata := archive.Metadata()
	metadata[0] = 'x'
	if got := archive.Metadata()[0]; got != '{' {
		t.Fatalf("Metadata() exposed mutable storage: first byte = %q", got)
	}
	chunk, err := archive.ReadChunk(1)
	if err != nil {
		t.Fatal(err)
	}
	chunk[0] = 'x'
	again, err := archive.ReadChunk(1)
	if err != nil || string(again) != "chunk-1" {
		t.Fatalf("ReadChunk() did not return independent bytes: %q, %v", again, err)
	}
	stats, err := archive.ReadEndStats()
	if err != nil || string(stats) != `{"result":"complete"}` {
		t.Fatalf("ReadEndStats() = %q, %v", stats, err)
	}
	if _, err := archive.ReadKeyFrame(99); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unlisted keyframe error = %v, want os.ErrNotExist", err)
	}
}

func TestOpenArchiveAllowsIncompleteGapsWithoutFullClaim(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "archive")
	game := Game{PlatformID: "KR", GameID: 55}
	a, err := createArchive(directory, game, defaultObserverResponseBytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.replaceVersion("2.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := a.replaceMetadata(testArchiveMetadata(t, game, 3, 1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if err := a.storePayload("chunks", 1, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := a.storePayload("chunks", 3, []byte("three")); err != nil {
		t.Fatal(err)
	}
	opened, err := OpenArchive(directory)
	if err != nil {
		t.Fatalf("OpenArchive() rejected valid partial archive: %v", err)
	}
	if opened.Manifest().Complete {
		t.Fatal("archive with a missing chunk was claimed complete")
	}
	if got := opened.Manifest().ChunkIDs; len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("partial IDs = %v, want [1 3]", got)
	}
}

func createCompleteTestArchive(t *testing.T, directory string, game Game, finalChunk, finalKeyFrame uint64) *Archive {
	t.Helper()
	archive, err := createArchive(directory, game, defaultObserverResponseBytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.replaceVersion("2.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := archive.replaceMetadata(testArchiveMetadata(t, game, finalChunk, finalKeyFrame, finalChunk, finalKeyFrame)); err != nil {
		t.Fatal(err)
	}
	for id := uint64(1); id <= finalChunk; id++ {
		if err := archive.storePayload("chunks", id, []byte("chunk-"+strconv.FormatUint(id, 10))); err != nil {
			t.Fatal(err)
		}
	}
	for id := uint64(1); id <= finalKeyFrame; id++ {
		if err := archive.storePayload("keyframes", id, []byte("keyframe-"+strconv.FormatUint(id, 10))); err != nil {
			t.Fatal(err)
		}
	}
	last := ChunkInfo{ChunkID: finalChunk, KeyFrameID: finalKeyFrame, EndGameChunkID: finalChunk}
	lastJSON, err := json.Marshal(last)
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.replaceLastChunkInfo(last, lastJSON); err != nil {
		t.Fatal(err)
	}
	manifest := archive.Manifest()
	manifest.LastChunk = last
	if err := archive.commitManifest(manifest); err != nil {
		t.Fatal(err)
	}
	if err := archive.writeEndStats([]byte(`{"result":"complete"}`)); err != nil {
		t.Fatal(err)
	}
	if err := archive.markComplete(finalChunk, finalKeyFrame, last); err != nil {
		t.Fatal(err)
	}
	return archive
}

func testArchiveMetadata(t *testing.T, game Game, lastChunk, lastKeyFrame, endChunk, endKeyFrame uint64) []byte {
	t.Helper()
	metadata := map[string]any{
		"gameKey":   map[string]any{"platformId": game.PlatformID, "gameId": game.GameID},
		"gameEnded": true, "lastChunkId": lastChunk, "lastKeyFrameId": lastKeyFrame,
		"endGameChunkId": endChunk, "endGameKeyFrameId": endKeyFrame,
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
