package recorder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

type replayProcess interface {
	pid() int
	exited() bool
	close() error
}
type desktop interface {
	acquire() (func(), error)
	launch(context.Context, Config, string) (replayProcess, error)
	selectPlayer(context.Context, int, uint16) error
}
type mediaVerifier interface {
	ready() error
	verify(context.Context, string, float64) error
}

// Recorder is reusable. At most one RecordFull call may run at a time. New does
// not launch the game, change settings, or open network connections.
type Recorder struct {
	config   Config
	api      *replayAPI
	desktop  desktop
	verifier mediaVerifier
	active   atomic.Bool
}

func New(config Config) (*Recorder, error) {
	for name, value := range map[string]time.Duration{"poll interval": config.PollInterval, "launch timeout": config.LaunchTimeout, "finalize timeout": config.FinalizeTimeout, "request timeout": config.RequestTimeout} {
		if value < 0 {
			return nil, fmt.Errorf("%s cannot be negative", name)
		}
	}
	if config.PollInterval == 0 {
		config.PollInterval = time.Second
	}
	if config.LaunchTimeout == 0 {
		config.LaunchTimeout = 2 * time.Minute
	}
	if config.FinalizeTimeout == 0 {
		config.FinalizeTimeout = 3 * time.Minute
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 5 * time.Second
	}
	if config.Logger == nil {
		config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if config.SelectionKeys == [10]uint16{} {
		config.SelectionKeys = [10]uint16{'1', '2', '3', '4', '5', 'Q', 'W', 'E', 'R', 'T'}
	}
	for _, key := range config.SelectionKeys {
		if key == 0 || key > 0xff {
			return nil, errors.New("SelectionKeys must contain ten Windows virtual-key codes")
		}
	}
	if config.ExtraLaunchArgs != nil {
		config.ExtraLaunchArgs = append(make([]string, 0, len(config.ExtraLaunchArgs)), config.ExtraLaunchArgs...)
	}
	api, err := newAPI(config.ReplayURL, config.RequestTimeout, config.StrictTLS)
	if err != nil {
		return nil, err
	}
	return &Recorder{config: config, api: api, desktop: nativeDesktop{}, verifier: probeVerifier{executable: config.ProbeExecutable}}, nil
}
func (r *Recorder) emit(stage Stage, current, total float64) {
	r.config.Logger.Debug("replay recording", "stage", stage, "current_seconds", current, "total_seconds", total)
	if r.config.OnProgress != nil {
		r.config.OnProgress(Progress{Stage: stage, CurrentSeconds: current, TotalSeconds: total})
	}
}
func wait(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func validateRequest(request Request) (Request, error) {
	id, err := ParseRiotID(request.Target.String())
	if err != nil {
		return request, err
	}
	request.Target = id
	if strings.ToLower(filepath.Ext(request.ReplayPath)) != ".rofl" {
		return request, errors.New("ReplayPath requires a .rofl file")
	}
	info, err := os.Stat(request.ReplayPath)
	if err != nil {
		return request, fmt.Errorf("replay: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return request, errors.New("replay must be a nonempty regular file")
	}
	request.ReplayPath, err = filepath.Abs(request.ReplayPath)
	if err != nil {
		return request, err
	}
	if strings.ToLower(filepath.Ext(request.OutputPath)) != ".webm" {
		return request, errors.New("OutputPath requires a .webm file")
	}
	request.OutputPath, err = filepath.Abs(request.OutputPath)
	if err != nil {
		return request, err
	}
	if _, err = os.Lstat(request.OutputPath); err == nil {
		return request, ErrOutputExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return request, err
	}
	parent, err := os.Stat(filepath.Dir(request.OutputPath))
	if err != nil || !parent.IsDir() {
		return request, errors.New("output directory must already exist")
	}
	if request.Width == 0 {
		request.Width = 1920
	}
	if request.Height == 0 {
		request.Height = 1080
	}
	if request.FPS == 0 {
		request.FPS = 60
	}
	if request.Width < 2 || request.Height < 2 || request.Width > 7680 || request.Height > 4320 || request.Width%2 != 0 || request.Height%2 != 0 || request.FPS < 1 || request.FPS > 120 {
		return request, errors.New("invalid dimensions or FPS (even dimensions up to 7680x4320, FPS 1–120)")
	}
	return request, nil
}

// RecordFull launches a local replay, locks its camera to Target, records 0..length,
// and verifies the output. It closes only the game process it launched. On failure
// it returns no successful Result and preserves partial output for inspection.
func (r *Recorder) RecordFull(ctx context.Context, request Request) (result Result, err error) {
	if !r.active.CompareAndSwap(false, true) {
		return Result{}, ErrBusy
	}
	defer r.active.Store(false)
	stage := StageValidate
	mayHaveOutput := false
	defer func() {
		if err != nil {
			partial := ""
			if info, e := os.Stat(request.OutputPath); mayHaveOutput && e == nil && info.Size() > 0 {
				partial = request.OutputPath
			}
			err = &Error{Stage: stage, Cause: err, PartialPath: partial}
			result = Result{}
		}
	}()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	r.emit(stage, 0, 0)
	request, err = validateRequest(request)
	if err != nil {
		return result, err
	}
	release, err := r.desktop.acquire()
	if err != nil {
		return result, err
	}
	defer release()
	if err = r.verifier.ready(); err != nil {
		return result, err
	}
	lock, err := os.OpenFile(request.OutputPath+".recorder-lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return result, fmt.Errorf("reserve output path: %w", err)
	}
	defer func() { _ = lock.Close(); _ = os.Remove(lock.Name()) }()
	var existing gameState
	if preflight := r.api.request(ctx, "GET", "/replay/game", nil, &existing); preflight != nil {
		if !errors.Is(preflight, syscall.ECONNREFUSED) {
			return result, fmt.Errorf("check existing replay client: %w", preflight)
		}
	} else if existing.PID > 0 {
		return result, ErrClientBusy
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	stage = StageLaunch
	r.emit(stage, 0, 0)
	process, err := r.desktop.launch(ctx, r.config, request.ReplayPath)
	if err != nil {
		return result, err
	}
	attemptedRecording := false
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), r.config.RequestTimeout)
		defer cancel()
		if attemptedRecording && !process.exited() {
			var owner gameState
			if e := r.api.request(cleanupCtx, "GET", "/replay/game", nil, &owner); e == nil && owner.PID == process.pid() {
				if e = r.api.request(cleanupCtx, "POST", "/replay/recording", map[string]any{"recording": false}, nil); e != nil {
					err = errors.Join(err, fmt.Errorf("stop recording: %w", e))
				}
			}
		}
		if e := process.close(); e != nil {
			err = errors.Join(err, fmt.Errorf("close owned replay process: %w", e))
		}
		r.api.close()
	}()
	stage = StageLoad
	r.emit(stage, 0, 0)
	loadCtx, cancelLoad := context.WithTimeout(ctx, r.config.LaunchTimeout)
	defer cancelLoad()
	var playback playbackState
	var players gameData
	for {
		if process.exited() {
			return result, errors.New("game exited before replay was ready; check patch compatibility")
		}
		var game gameState
		if e := r.api.request(loadCtx, "GET", "/replay/game", nil, &game); e == nil {
			if game.PID != process.pid() {
				return result, errors.New("Replay API belongs to another process")
			}
			if e = r.api.request(loadCtx, "GET", "/replay/playback", nil, &playback); e == nil && finitePositive(playback.Length) && !playback.Seeking {
				if e = r.api.request(loadCtx, "GET", "/liveclientdata/allgamedata", nil, &players); e == nil && len(players.Players) > 0 {
					break
				}
			}
		}
		if err = wait(loadCtx, r.config.PollInterval); err != nil {
			return result, err
		}
	}
	var previous recordingState
	if err = r.api.request(loadCtx, "GET", "/replay/recording", nil, &previous); err != nil {
		return result, err
	}
	if previous.Recording == nil {
		return result, errors.New("Replay API omits recording state")
	}
	if *previous.Recording {
		return result, ErrClientBusy
	}
	// Seek and pause before targeting, so no game content is lost during setup.
	if err = r.api.request(loadCtx, "POST", "/replay/playback", map[string]any{"time": 0, "paused": true, "speed": 1}, nil); err != nil {
		return result, err
	}
	for {
		if err = r.api.request(loadCtx, "GET", "/replay/playback", nil, &playback); err != nil {
			return result, err
		}
		if !playback.Seeking && playback.Paused && playback.Time <= 0.25 {
			break
		}
		if err = wait(loadCtx, r.config.PollInterval); err != nil {
			return result, err
		}
	}
	length := playback.Length
	if !finitePositive(length) || length > 24*60*60 {
		return result, errors.New("invalid replay length")
	}
	index, target, e := locateTarget(players.Players, request.Target)
	if e != nil {
		return result, e
	}
	stage = StageTarget
	r.emit(stage, 0, length)
	verified := false
	for attempt := 0; attempt < 5; attempt++ {
		if err = r.desktop.selectPlayer(loadCtx, process.pid(), r.config.SelectionKeys[index]); err != nil {
			return result, err
		}
		if err = wait(loadCtx, r.config.PollInterval); err != nil {
			return result, err
		}
		var render renderState
		if err = r.api.request(loadCtx, "GET", "/replay/render", nil, &render); err != nil {
			return result, err
		}
		if locked(render, target, request.Target) {
			verified = true
			break
		}
	}
	if !verified {
		return result, ErrCameraLock
	}
	if _, e = os.Lstat(request.OutputPath); e == nil {
		return result, ErrOutputExists
	} else if !errors.Is(e, os.ErrNotExist) {
		return result, e
	}
	stage = StageRecord
	r.emit(stage, 0, length)
	start := time.Now().UTC()
	attemptedRecording = true // Even a failed POST can have reached the game.
	mayHaveOutput = true
	options := map[string]any{"recording": true, "path": request.OutputPath, "codec": "webm", "startTime": 0, "endTime": length, "width": request.Width, "height": request.Height, "framesPerSecond": request.FPS, "enforceFrameRate": true, "replaySpeed": 1}
	if err = r.api.request(ctx, "POST", "/replay/recording", options, nil); err != nil {
		return result, err
	}
	if err = r.api.request(ctx, "POST", "/replay/playback", map[string]any{"paused": false, "speed": 1}, nil); err != nil {
		return result, err
	}
	recordingCtx, cancelRecord := context.WithTimeout(ctx, time.Duration(length*1.5*float64(time.Second))+2*time.Minute)
	defer cancelRecord()
	started := false
	startDeadline := time.Now().Add(15 * time.Second)
	for {
		if process.exited() {
			return result, errors.New("game exited during recording")
		}
		var owner gameState
		if err = r.api.request(recordingCtx, "GET", "/replay/game", nil, &owner); err != nil {
			return result, err
		}
		if owner.PID != process.pid() {
			return result, errors.New("Replay API ownership changed during recording")
		}
		var state recordingState
		if err = r.api.request(recordingCtx, "GET", "/replay/recording", nil, &state); err != nil {
			return result, err
		}
		if state.Recording == nil {
			return result, errors.New("Replay API omits recording state")
		}
		if !started && !*state.Recording && state.Path == "" {
			if time.Now().After(startDeadline) {
				return result, ErrRecordingIncomplete
			}
			if err = wait(recordingCtx, r.config.PollInterval); err != nil {
				return result, err
			}
			continue
		}
		if state.Path == "" || !samePath(state.Path, request.OutputPath) || math.Abs(state.Start) > 0.25 || math.Abs(state.End-length) > 0.5 || !finitePositive(state.End) || math.IsNaN(state.Current) || math.IsInf(state.Current, 0) {
			return result, errors.New("Replay API recording range or output differs from request")
		}
		var render renderState
		if err = r.api.request(recordingCtx, "GET", "/replay/render", nil, &render); err != nil {
			return result, err
		}
		if !locked(render, target, request.Target) {
			return result, ErrCameraLock
		}
		if *state.Recording {
			started = true
		}
		r.emit(stage, state.Current, length)
		if !*state.Recording {
			if !started && state.Current < length-0.5 && time.Now().Before(startDeadline) {
				if err = wait(recordingCtx, r.config.PollInterval); err != nil {
					return result, err
				}
				continue
			}
			if !started || state.Current < length-0.5 {
				return result, ErrRecordingIncomplete
			}
			break
		}
		if err = wait(recordingCtx, r.config.PollInterval); err != nil {
			return result, err
		}
	}
	attemptedRecording = false
	stage = StageFinalize
	r.emit(stage, length, length)
	finalizeCtx, cancelFinalize := context.WithTimeout(ctx, r.config.FinalizeTimeout)
	defer cancelFinalize()
	var lastSize int64
	stable := 0
	for {
		info, e := os.Stat(request.OutputPath)
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return result, e
		}
		if e == nil && info.Mode().IsRegular() && info.Size() > 0 {
			if info.Size() == lastSize {
				stable++
			} else {
				stable = 0
			}
			lastSize = info.Size()
			if stable >= 2 {
				break
			}
		}
		if err = wait(finalizeCtx, r.config.PollInterval); err != nil {
			return result, err
		}
	}
	if err = r.verifier.verify(finalizeCtx, request.OutputPath, length); err != nil {
		return result, err
	}
	return Result{Path: request.OutputPath, Target: request.Target, DurationSeconds: length, StartedAt: start, FinishedAt: time.Now().UTC()}, nil
}
func samePath(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }
