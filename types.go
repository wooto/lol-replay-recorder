package recorder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

var (
	ErrUnsupportedPlatform = errors.New("recording requires Windows")
	ErrBusy                = errors.New("another recording is using the local replay client")
	ErrTargetNotFound      = errors.New("target Riot ID not found in replay")
	ErrAmbiguousTarget     = errors.New("target Riot ID is ambiguous")
	ErrCameraLock          = errors.New("target camera lock could not be verified")
	ErrOutputExists        = errors.New("output already exists")
	ErrClientBusy          = errors.New("a replay client is already running")
	ErrRecordingIncomplete = errors.New("recording did not cover the full replay")
)

// RiotID identifies a player by their complete game name and tag line.
type RiotID struct {
	GameName string
	TagLine  string
}

func ParseRiotID(value string) (RiotID, error) {
	name, tag, ok := strings.Cut(strings.TrimSpace(value), "#")
	id := RiotID{GameName: strings.TrimSpace(name), TagLine: strings.TrimSpace(tag)}
	if !ok || id.GameName == "" || id.TagLine == "" || strings.Contains(tag, "#") {
		return RiotID{}, errors.New("target must be a full Riot ID: gameName#tagLine")
	}
	return id, nil
}
func (id RiotID) String() string { return id.GameName + "#" + id.TagLine }

type Stage string

const (
	StageValidate Stage = "validate"
	StageLaunch   Stage = "launch"
	StageLoad     Stage = "load"
	StageTarget   Stage = "target"
	StageRecord   Stage = "record"
	StageFinalize Stage = "finalize"
)

// Progress callbacks run synchronously. They must return promptly and must not
// call RecordFull on the same recorder.
type Progress struct {
	Stage          Stage
	CurrentSeconds float64
	TotalSeconds   float64
}

// Config controls the local client. The zero value selects documented defaults.
// ExtraLaunchArgs override the default launch arguments when non-nil.
type Config struct {
	GameExecutable string
	// LaunchReplay optionally launches through the calling application's client
	// integration. It must return only the process it launched, never an existing
	// game, and honor context cancellation. On failure it must clean up any process
	// it started.
	LaunchReplay func(context.Context, string) (ReplayProcess, error)
	// ProbeExecutable defaults to ffprobe. It validates the finalized WebM.
	ProbeExecutable string
	ExtraLaunchArgs []string
	ReplayURL       string
	PollInterval    time.Duration
	LaunchTimeout   time.Duration
	FinalizeTimeout time.Duration
	RequestTimeout  time.Duration
	// SelectionKeys are Windows virtual-key codes in ORDER then CHAOS team order.
	// Zero means standard 1–5/Q–T bindings; game settings are never rewritten.
	SelectionKeys [10]uint16
	// StrictTLS requires the endpoint certificate to be trusted by the OS.
	// Default transport tolerates the game certificate only on numeric loopback.
	StrictTLS  bool
	Logger     *slog.Logger
	OnProgress func(Progress)
}

// ReplayProcess is an owned game started by Config.LaunchReplay. Close must
// terminate only this process and release its resources. Methods must be safe
// to call while the game exits; PID must remain stable and positive.
type ReplayProcess interface {
	PID() int
	Exited() bool
	Close() error
}

// Request records the whole replay at normal speed. Only WebM output is supported
// in this initial version; transcoding is the caller's responsibility.
type Request struct {
	ReplayPath string
	Target     RiotID
	OutputPath string
	Width      int
	Height     int
	FPS        int
}
type Result struct {
	Path            string    `json:"path"`
	Target          RiotID    `json:"target"`
	DurationSeconds float64   `json:"duration_seconds"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
}

// Error retains the failing stage and any incomplete output. PartialPath is never
// a successful FULL result. errors.Is works for context and sentinel errors.
type Error struct {
	Stage       Stage
	Cause       error
	PartialPath string
}

func (e *Error) Error() string { return fmt.Sprintf("recorder %s: %v", e.Stage, e.Cause) }
func (e *Error) Unwrap() error { return e.Cause }
