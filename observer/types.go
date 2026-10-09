package observer

import (
	"errors"
	"net/http"
	"time"
)

var (
	// ErrArchiveIncomplete means the server announced the end of a match but
	// one or more expected archive files are unavailable.
	ErrArchiveIncomplete = errors.New("observer archive is incomplete")
	// ErrOutputExists means Capture refused to overwrite its destination directory.
	ErrOutputExists = errors.New("archive destination already exists")
)

// Game identifies a match on an observer server.
type Game struct {
	PlatformID string `json:"platformId"`
	GameID     uint64 `json:"gameId"`
}

// ClientConfig controls an experimental observer protocol client. BaseURL must
// be an explicit HTTP or HTTPS URL. Zero polling and response limits select
// bounded defaults.
type ClientConfig struct {
	BaseURL          string
	HTTPClient       *http.Client
	PollInterval     time.Duration
	MaxResponseBytes int64
}

// ChunkInfo is the observer protocol's latest chunk snapshot.
type ChunkInfo struct {
	ChunkID            uint64 `json:"chunkId"`
	KeyFrameID         uint64 `json:"keyFrameId"`
	NextChunkID        uint64 `json:"nextChunkId"`
	EndStartupChunkID  uint64 `json:"endStartupChunkId"`
	StartGameChunkID   uint64 `json:"startGameChunkId"`
	EndGameChunkID     uint64 `json:"endGameChunkId"`
	Duration           int64  `json:"duration"`
	AvailableSince     int64  `json:"availableSince"`
	NextAvailableChunk int64  `json:"nextAvailableChunk"`
}

// Manifest describes the files stored in an observer archive. Complete is
// true only after the observer server announced an end and every expected
// chunk and keyframe was downloaded and saved as a readable, bounded file.
type Manifest struct {
	FormatVersion int       `json:"formatVersion"`
	Game          Game      `json:"game"`
	Complete      bool      `json:"complete"`
	ChunkIDs      []uint64  `json:"chunkIds"`
	KeyFrameIDs   []uint64  `json:"keyFrameIds"`
	LastChunk     ChunkInfo `json:"lastChunk"`
}
