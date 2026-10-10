package lcu

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	recorder "github.com/wooto/lol-replay-recorder"
	"github.com/wooto/lol-replay-recorder/spectator"
)

type LaunchConfig struct {
	ReplayURL    string
	PollInterval time.Duration
	Timeout      time.Duration
	StrictTLS    bool
}

var launchMu sync.Mutex

// LaunchReplay bridges LCU Watch to recorder.Config.LaunchReplay on Windows.
// It requires an idle client and no existing game. The new game is identified
// through both a process snapshot and Replay API PID; failures close only the
// uniquely identified new process. Serialize user/manual game launches too.
func (c *Client) LaunchReplay(ctx context.Context, replay Replay, config LaunchConfig) (recorder.ReplayProcess, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	if !launchMu.TryLock() {
		return nil, ErrClientBusy
	}
	defer launchMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if replay.GameID == 0 || !filepath.IsAbs(replay.Path) {
		return nil, errors.New("downloaded replay ID and absolute path required")
	}
	info, err := os.Stat(replay.Path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, ErrReplayFile
	}
	region, err := c.Region(ctx)
	if err != nil {
		return nil, err
	}
	var dir string
	if err = c.http.Request(ctx, "GET", "/lol-replays/v1/rofls/path", nil, &dir); err != nil {
		return nil, err
	}
	expected := filepath.Join(dir, region.Region+"-"+strconv.FormatUint(replay.GameID, 10)+".rofl")
	if !filepath.IsAbs(dir) || filepath.Clean(expected) != filepath.Clean(replay.Path) {
		return nil, ErrReplayFile
	}
	if config.PollInterval < 0 || config.Timeout < 0 {
		return nil, errors.New("launch timing cannot be negative")
	}
	poll := config.PollInterval
	if poll == 0 {
		poll = 100 * time.Millisecond
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = time.Minute
	}
	api, err := spectator.New(spectator.Config{BaseURL: config.ReplayURL, StrictTLS: config.StrictTLS})
	if err != nil {
		return nil, err
	}
	defer api.Close()
	before, err := gameProcesses()
	if err != nil {
		return nil, err
	}
	if len(before) > 0 {
		return nil, ErrClientBusy
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err = api.Game(ctx); err == nil {
		return nil, ErrClientBusy
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err = c.Watch(ctx, replay.GameID); err != nil {
		return nil, err
	}
	var owned recorder.ReplayProcess
	success := false
	defer func() {
		if !success && owned != nil {
			_ = owned.Close()
		}
	}()
	for {
		processes, err := gameProcesses()
		if err != nil {
			return nil, err
		}
		if len(processes) > 1 {
			return nil, errors.New("multiple new game processes; ownership is ambiguous")
		}
		if owned == nil && len(processes) == 1 {
			for pid := range processes {
				owned, err = openGameProcess(pid)
			}
			if err != nil {
				return nil, err
			}
		}
		game, apiErr := api.Game(ctx)
		if apiErr == nil && owned != nil {
			if game.ProcessID != owned.PID() {
				return nil, errors.New("Replay API process does not match newly launched game")
			}
			if owned.Exited() {
				return nil, errors.New("launched game exited")
			}
			success = true
			return owned, nil
		}
		if owned != nil && owned.Exited() {
			return nil, errors.New("launched game exited before Replay API became ready")
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
