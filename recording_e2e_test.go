//go:build e2e && windows && amd64

package recorder_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	recorder "github.com/wooto/lol-replay-recorder"
)

// Explicit opt-in: go test -tags=e2e -run TestLiveFullRecording -timeout 3h.
// No real game is launched by the normal test suite or public PR CI.
func TestLiveFullRecording(t *testing.T) {
	replay := os.Getenv("LOL_REPLAY_PATH")
	target := os.Getenv("LOL_REPLAY_TARGET")
	if replay == "" || target == "" {
		t.Fatal("set LOL_REPLAY_PATH and LOL_REPLAY_TARGET for this explicit live test")
	}
	id, err := recorder.ParseRiotID(target)
	if err != nil {
		t.Fatal(err)
	}
	client, err := recorder.New(recorder.Config{GameExecutable: os.Getenv("LOL_GAME_EXECUTABLE"), ProbeExecutable: os.Getenv("LOL_FFPROBE"), OnProgress: func(p recorder.Progress) { t.Logf("%s %.1f/%.1fs", p.Stage, p.CurrentSeconds, p.TotalSeconds) }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	result, err := client.RecordFull(ctx, recorder.Request{ReplayPath: replay, Target: id, OutputPath: filepath.Join(t.TempDir(), "full.webm")})
	if err != nil {
		t.Fatal(err)
	}
	if result.DurationSeconds <= 0 || result.Path == "" {
		t.Fatalf("invalid full result: %+v", result)
	}
}
