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
	"runtime"
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

type customProcess struct{ ReplayProcess }

func (p customProcess) pid() int     { return p.PID() }
func (p customProcess) exited() bool { return p.Exited() }
func (p customProcess) close() error { return p.Close() }

func (r *Recorder) launch(ctx context.Context, path string) (replayProcess, error) {
	if r.config.LaunchReplay == nil {
		return r.desktop.launch(ctx, r.config, path)
	}
	process, err := r.config.LaunchReplay(ctx, path)
	if err != nil {
		return nil, err
	}
	if process == nil {
		return nil, errors.New("LaunchReplay returned no process")
	}
	if process.PID() <= 0 {
		return nil, errors.Join(errors.New("LaunchReplay returned an invalid process ID"), process.Close())
	}
	return customProcess{process}, nil
}

type desktop interface {
	acquire() (func(), error)
	launch(context.Context, Config, string) (replayProcess, error)
	selectPlayer(context.Context, int, uint16) error
}
type mediaVerifier interface {
	ready() error
	verify(context.Context, string, float64, int, int) (probeObservation, error)
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
	if config.ConfigureHotkeys && config.Hotkeys == nil {
		return nil, errors.New("ConfigureHotkeys requires a Hotkeys settings adapter")
	}
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
	if err := validateSlotKeys(config.SelectionKeys); err != nil {
		return nil, err
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

func (r *Recorder) waitForCameraOffset(ctx context.Context, target player, id RiotID, expected cameraVector, gameTime float64) (renderState, bool, error) {
	ackCtx, cancel := context.WithTimeout(ctx, cameraOffsetAckTimeout)
	defer cancel()
	var latest renderState
	for attempt := 0; attempt < cameraOffsetAckAttempts; attempt++ {
		if err := r.api.request(ackCtx, "GET", "/replay/render", nil, &latest); err != nil {
			if ctx.Err() != nil {
				return latest, false, ctx.Err()
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return latest, false, cameraOffsetAckError(gameTime, latest.SelectionOffset, expected)
			}
			return latest, false, err
		}
		if !cameraPoseValid(latest) || !cameraOffsetWithinRange(latest) || !cameraInputControlsValid(latest) {
			return latest, false, fmt.Errorf("%w at %.3fs (camera profile or FPS input controls changed while awaiting offset readback %+v, expected %+v)", ErrCameraLock, gameTime, latest.SelectionOffset, expected)
		}
		if latest.SelectionName == "" && latest.CameraAttached != nil && *latest.CameraAttached {
			return latest, true, nil
		}
		if !locked(latest, target, id) {
			return latest, false, fmt.Errorf("%w at %.3fs (selection %q while awaiting camera offset readback)", ErrCameraLock, gameTime, latest.SelectionName)
		}
		if cameraOffsetMatches(*latest.SelectionOffset, expected) {
			return latest, false, nil
		}
		if attempt+1 == cameraOffsetAckAttempts {
			return latest, false, cameraOffsetAckError(gameTime, latest.SelectionOffset, expected)
		}
		if err := wait(ackCtx, cameraOffsetAckInterval); err != nil {
			if ctx.Err() != nil {
				return latest, false, ctx.Err()
			}
			return latest, false, cameraOffsetAckError(gameTime, latest.SelectionOffset, expected)
		}
	}
	return latest, false, cameraOffsetAckError(gameTime, latest.SelectionOffset, expected)
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
	return r.record(ctx, request, 0, 0, true)
}

// RecordInterval records the replay match-time interval [fromSeconds,toSeconds].
// The output is normalized to a zero-based timeline and must cover the full
// requested duration. Bounds are checked against the loaded replay.
func (r *Recorder) RecordInterval(ctx context.Context, request Request, fromSeconds, toSeconds float64) (result Result, err error) {
	if math.IsNaN(fromSeconds) || math.IsInf(fromSeconds, 0) || math.IsNaN(toSeconds) || math.IsInf(toSeconds, 0) || fromSeconds < 0 || toSeconds <= fromSeconds {
		return Result{}, errors.New("recording interval requires finite bounds with 0 <= from < to")
	}
	return r.record(ctx, request, fromSeconds, toSeconds, false)
}

func (r *Recorder) record(ctx context.Context, request Request, fromSeconds, toSeconds float64, full bool) (result Result, err error) {
	if !r.active.CompareAndSwap(false, true) {
		return Result{}, ErrBusy
	}
	defer r.active.Store(false)
	defer r.api.close()
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
		if !connectionRefused(preflight) {
			return result, fmt.Errorf("check existing replay client: %w", preflight)
		}
	} else if existing.PID > 0 {
		return result, ErrClientBusy
	}
	if r.config.Hotkeys != nil {
		keys, checkErr := r.config.Hotkeys.Read(ctx)
		if checkErr != nil {
			return result, errors.Join(ErrHotkeySettings, checkErr)
		}
		if checkErr = validateSlotKeys(keys); checkErr != nil {
			return result, checkErr
		}
		if keys != r.config.SelectionKeys {
			if !r.config.ConfigureHotkeys {
				return result, fmt.Errorf("%w: effective bindings differ from SelectionKeys", ErrHotkeySettings)
			}
			if err = ctx.Err(); err != nil {
				return result, err
			}
			if checkErr = r.config.Hotkeys.Backup(ctx); checkErr != nil {
				return result, errors.Join(ErrHotkeySettings, fmt.Errorf("back up bindings: %w", checkErr))
			}
			if err = ctx.Err(); err != nil {
				return result, err
			}
			if checkErr = r.config.Hotkeys.Apply(ctx, r.config.SelectionKeys); checkErr != nil {
				return result, errors.Join(ErrHotkeySettings, fmt.Errorf("apply bindings: %w", checkErr))
			}
			keys, checkErr = r.config.Hotkeys.Read(ctx)
			if checkErr != nil {
				return result, errors.Join(ErrHotkeySettings, checkErr)
			}
			if checkErr = validateSlotKeys(keys); checkErr != nil {
				return result, checkErr
			}
			if keys != r.config.SelectionKeys {
				return result, fmt.Errorf("%w: bindings still differ after applying settings", ErrHotkeySettings)
			}
		}
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	stage = StageLaunch
	r.emit(stage, 0, 0)
	process, err := r.launch(ctx, request.ReplayPath)
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
	// At exact time zero current clients have not created selectable champions.
	// Prepare at 0.1; full capture and explicit intervals each apply their
	// calibrated native start and validate decoded video against the requested range.
	if err = r.api.request(loadCtx, "POST", "/replay/playback", map[string]any{"time": 0.1, "paused": true, "speed": 1}, nil); err != nil {
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
	if full {
		fromSeconds, toSeconds = 0, length
	} else if toSeconds > length {
		return result, errors.New("recording interval exceeds replay length")
	}
	duration := toSeconds - fromSeconds
	if !finitePositive(duration) {
		return result, errors.New("invalid recording interval duration")
	}
	index, target, e := locateTarget(players.Players, request.Target)
	if e != nil {
		return result, e
	}
	stage = StageTarget
	r.emit(stage, 0, length)
	verified := false
	for attempt := 0; attempt < 5; attempt++ {
		var render renderState
		render, err = r.selectTarget(loadCtx, process.pid(), r.config.SelectionKeys[index])
		if err != nil {
			return result, err
		}
		if locked(render, target, request.Target) {
			verified = true
			break
		}
	}
	if !verified {
		if !r.config.RecoverFocus {
			return result, ErrCameraLock
		}
		if _, err = r.recoverFocus(loadCtx, process.pid(), r.config.SelectionKeys[index], target, request.Target, playback.Time); err != nil {
			return result, err
		}
	}
	// Use the client's normal 56-degree elevated view; hotkey selection and
	// attachment alone can otherwise report success from inside terrain.
	offset := baseCameraOffset
	rotation := baseCameraRotation
	if err = r.api.request(loadCtx, "POST", "/replay/render", map[string]any{
		"cameraMode": "fps", "selectionOffset": offset, "cameraRotation": rotation,
		"cameraLockX": false, "cameraLockY": false, "cameraLockZ": false,
		"cameraMoveSpeed": 0, "cameraLookSpeed": 0,
	}, nil); err != nil {
		return result, err
	}
	// Preserve the camera angle on the render sequence. Target selection remains
	// a hotkey action, with identity verified through render readback.
	if err = r.api.request(loadCtx, "POST", "/replay/sequence", map[string]any{
		"selectionOffset": []cameraVector{},
		"cameraRotation": []map[string]any{
			{"time": 0, "value": rotation, "blend": "snap"},
			{"time": length, "value": rotation, "blend": "snap"},
		},
	}, nil); err != nil {
		return result, err
	}
	var prepared renderState
	if err = r.api.request(loadCtx, "GET", "/replay/render", nil, &prepared); err != nil {
		return result, err
	}
	if !elevatedCamera(prepared) || !cameraInputControlsValid(prepared) || !locked(prepared, target, request.Target) || !cameraOffsetMatches(*prepared.SelectionOffset, baseCameraOffset) {
		return result, ErrCameraLock
	}
	follower := &cameraFollower{}
	if _, ok := follower.follow(prepared, playback.Time, time.Now()); !ok {
		return result, ErrCameraLock
	}
	expectedCameraOffset := baseCameraOffset
	if _, e = os.Lstat(request.OutputPath); e == nil {
		return result, ErrOutputExists
	} else if !errors.Is(e, os.ErrNotExist) {
		return result, e
	}
	const nativePreroll = 5.0
	// Explicit intervals start at their requested match-time boundary. The
	// negative five-second workaround is retained only for RecordFull, whose
	// zero-based output has been calibrated to need it on the tested client.
	nativeStart := fromSeconds
	if full {
		nativeStart -= nativePreroll
	} else if fromSeconds == 0 && toSeconds > 0.1 {
		// The client has no champion objects at exact zero. Reuse the warmed
		// preflight position to avoid its zero-time reload; verify actual packet
		// bounds against the requested zero-based interval before accepting.
		nativeStart = 0.1
	}
	encoderStart := math.Max(0, nativeStart)
	seekTime := math.Max(0.1, nativeStart)
	stage = StageRecord
	r.emit(stage, 0, duration)
	// Seek to the native output start so recording startup does not rewind the
	// game after target setup.
	if seekTime > 0.25 {
		if err = r.api.request(loadCtx, "POST", "/replay/playback", map[string]any{"time": seekTime, "paused": true, "speed": 1}, nil); err != nil {
			return result, err
		}
		for {
			if err = r.api.request(loadCtx, "GET", "/replay/playback", nil, &playback); err != nil {
				return result, err
			}
			if !playback.Seeking && playback.Paused && math.Abs(playback.Time-seekTime) <= 0.25 {
				break
			}
			if err = wait(loadCtx, r.config.PollInterval); err != nil {
				return result, err
			}
		}
		// Seeking changes the selected target's world position independently
		// of the time-zero camera sample used during setup. Rebase following on
		// the first verified render frame at the interval start.
		follower.suspend()
	}
	if r.config.RecoverFocus {
		// A seek can destroy the selected champion object. Repair after the
		// final seek, not only before it, and allow the UI/camera to settle.
		settleCtx, cancel := context.WithTimeout(loadCtx, 25*time.Second)
		stable := time.Time{}
		for {
			var render renderState
			if err = r.api.request(settleCtx, "GET", "/replay/playback", nil, &playback); err != nil {
				cancel()
				return result, err
			}
			if err = r.api.request(settleCtx, "GET", "/replay/render", nil, &render); err != nil {
				cancel()
				return result, err
			}
			if playback.Seeking || !playback.Paused || math.Abs(playback.Time-seekTime) > 0.25 {
				stable = time.Time{}
			} else if focusDrift(render, target, request.Target, baseCameraOffset, true) {
				stable = time.Time{}
				if _, err = r.recoverFocus(settleCtx, process.pid(), r.config.SelectionKeys[index], target, request.Target, playback.Time); err != nil {
					cancel()
					return result, err
				}
			} else if stable.IsZero() {
				stable = time.Now()
			} else if time.Since(stable) >= 2*time.Second {
				break
			}
			if err = wait(settleCtx, 100*time.Millisecond); err != nil {
				cancel()
				return result, err
			}
		}
		cancel()
		follower.suspend()
		expectedCameraOffset = baseCameraOffset
	}
	start := time.Now().UTC()
	// League Director starts playback before enabling the recorder. Afterwards
	// the encoder owns playback timing to enforce the requested frame rate.
	if err = r.api.request(ctx, "POST", "/replay/playback", map[string]any{"paused": false}, nil); err != nil {
		return result, err
	}
	attemptedRecording = true // Even a failed POST can have reached the game.
	mayHaveOutput = true
	// Real-time capture preserves match timing on the current client. Its
	// accelerated frame-enforced mode produced shortened videos in live tests.
	// In the tested client, FPS capture begins five seconds after startTime.
	// Negative pre-roll initializes capture before game time zero; decoded media
	// timestamps must prove the exact requested duration on a zero-based timeline.
	options := map[string]any{"recording": true, "path": request.OutputPath, "codec": "webm", "startTime": nativeStart, "endTime": toSeconds, "width": request.Width, "height": request.Height, "framesPerSecond": request.FPS, "enforceFrameRate": false, "replaySpeed": 1}
	if err = r.api.request(ctx, "POST", "/replay/recording", options, nil); err != nil {
		return result, err
	}
	recordingCtx, cancelRecord := context.WithTimeout(ctx, time.Duration(duration*1.5*float64(time.Second))+2*time.Minute)
	defer cancelRecord()
	started := false
	startDeadline := time.Now().Add(15 * time.Second)
	lastProgressAt := time.Now()
	var focusRecoveries []time.Time
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
		// Current clients clear recording parameters after the encoder finishes.
		// A reset is only a completion candidate after recording was observed;
		// require playback at the known end, then validate the entire output below.
		if started && !*state.Recording && state.Path == "" && state.Start == -1 && state.End == -1 && state.Current == 0 {
			var ended playbackState
			if err = r.api.request(recordingCtx, "GET", "/replay/playback", nil, &ended); err != nil {
				return result, err
			}
			if ended.Seeking || !finitePositive(ended.Time) || !finitePositive(ended.Length) || math.Abs(ended.Length-length) > 0.5 || ended.Time < toSeconds-0.5 {
				return result, ErrRecordingIncomplete
			}
			state.Path, state.Start, state.End, state.Current = request.OutputPath, nativeStart, toSeconds, toSeconds
		}
		// Native completion retains the path/current time but resets endTime to
		// -1. Preserve all normal path, start, target-camera and media checks.
		if started && !*state.Recording && state.End == -1 && state.Current >= toSeconds-0.5 {
			state.End = toSeconds
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
		if state.Path == "" || !samePath(state.Path, request.OutputPath) || math.Abs(state.Start-nativeStart) > 0.25 || math.Abs(state.End-toSeconds) > 0.5 || !finitePositive(state.End) || math.IsNaN(state.Current) || math.IsInf(state.Current, 0) {
			return result, errors.New("Replay API recording range or output differs from request")
		}
		var render renderState
		if err = r.api.request(recordingCtx, "GET", "/replay/render", nil, &render); err != nil {
			return result, err
		}
		preRoll := state.Current < fromSeconds
		selectionLocked := locked(render, target, request.Target)
		if preRoll {
			// Native pre-roll is before the requested video interval. Keep proving
			// the target selection, but rebase camera following at the first frame
			// in the requested interval instead of comparing pre-roll offsets.
			follower.suspend()
		}
		lifecycleReset := follower.requiresLifecycleReset(state.Current)
		// Keep watching throughout capture. Correct a changed player, detached
		// camera, invalid view or unexpected offset before the fatal checks below.
		// Empty selections retain the existing death/respawn handling.
		confirmedDeath := false
		if r.config.RecoverFocus && render.SelectionName == "" {
			var live gameData
			if err = r.api.request(recordingCtx, "GET", "/liveclientdata/allgamedata", nil, &live); err != nil {
				return result, err
			}
			_, current, e := locateTarget(live.Players, request.Target)
			confirmedDeath = e == nil && current.Team == target.Team && current.IsDead != nil && *current.IsDead && live.Clock.Time != nil && finiteNumber(*live.Clock.Time) && math.Abs(*live.Clock.Time-state.Current) <= 2
		}
		if r.config.RecoverFocus && !confirmedDeath && focusDrift(render, target, request.Target, expectedCameraOffset, !preRoll && !lifecycleReset) {
			now := time.Now()
			for len(focusRecoveries) > 0 && now.Sub(focusRecoveries[0]) > 30*time.Second {
				focusRecoveries = focusRecoveries[1:]
			}
			if len(focusRecoveries) >= 5 {
				return result, fmt.Errorf("%w: focus unstable after five recoveries in 30 seconds", ErrCameraLock)
			}
			focusRecoveries = append(focusRecoveries, now)
			render, err = r.recoverFocus(recordingCtx, process.pid(), r.config.SelectionKeys[index], target, request.Target, state.Current)
			if err != nil {
				return result, err
			}
			follower.suspend()
			expectedCameraOffset = baseCameraOffset
			selectionLocked = locked(render, target, request.Target)
			lifecycleReset = true
		}
		if !cameraPoseValid(render) {
			return result, fmt.Errorf("%w at %.3fs (camera view changed)", ErrCameraLock, state.Current)
		}
		if !cameraInputControlsValid(render) {
			return result, fmt.Errorf("%w at %.3fs (FPS camera input controls changed)", ErrCameraLock, state.Current)
		}
		if !preRoll && selectionLocked && !lifecycleReset && !cameraOffsetMatches(*render.SelectionOffset, expectedCameraOffset) {
			return result, fmt.Errorf("%w at %.3fs (between-update camera follow offset readback %+v, expected %+v)", ErrCameraLock, state.Current, *render.SelectionOffset, expectedCameraOffset)
		}
		if !preRoll && !cameraOffsetWithinRange(render) && !(lifecycleReset && selectionLocked) {
			return result, fmt.Errorf("%w at %.3fs (camera view changed)", ErrCameraLock, state.Current)
		}
		if !locked(render, target, request.Target) {
			follower.suspend()
			// Starting the encoder can temporarily remove game objects. Recover an
			// empty selection only at native preroll or requested video start; a
			// different selected player or any later lock loss remains fatal.
			// Native playback clocks can report a frame just before the configured
			// boundary. Treat that narrow edge as startup so transient empty target
			// selection can be restored before the requested interval begins.
			atEncoderStart := state.Current >= encoderStart-0.25 && state.Current <= encoderStart+0.25
			atVideoStart := state.Current >= fromSeconds-0.25 && state.Current <= fromSeconds+0.25
			if (atEncoderStart || atVideoStart) && render.SelectionName == "" && time.Now().Before(startDeadline) {
				render, err = r.selectTarget(recordingCtx, process.pid(), r.config.SelectionKeys[index])
				if err != nil {
					return result, err
				}
				if !locked(render, target, request.Target) && render.SelectionName != "" {
					return result, fmt.Errorf("%w at startup (selected %q)", ErrCameraLock, render.SelectionName)
				}
				continue
			}
			if preRoll {
				return result, fmt.Errorf("%w during interval pre-roll at %.3fs", ErrCameraLock, state.Current)
			}
			// A dead champion can disappear as a selectable object. Permit an
			// empty attached selection only when live data confirms this same target
			// is dead. The next alive frame must verify hotkey selection again.
			var live gameData
			if render.SelectionName != "" || render.CameraAttached == nil || !*render.CameraAttached {
				return result, fmt.Errorf("%w at %.3fs (selection %q)", ErrCameraLock, state.Current, render.SelectionName)
			}
			// Render and live-data snapshots can straddle a respawn. Re-focus the
			// owned game and select the same team slot through its verified hotkey.
			render, err = r.selectTarget(recordingCtx, process.pid(), r.config.SelectionKeys[index])
			if err != nil {
				return result, err
			}
			if locked(render, target, request.Target) {
				lifecycleReset = follower.requiresLifecycleReset(state.Current)
				selectionLocked = locked(render, target, request.Target)
				if !cameraPoseValid(render) || !cameraInputControlsValid(render) || (!cameraOffsetWithinRange(render) && !(lifecycleReset && selectionLocked)) {
					return result, ErrCameraLock
				}
			}
			if !locked(render, target, request.Target) {
				if render.SelectionName != "" || render.CameraAttached == nil || !*render.CameraAttached {
					return result, ErrCameraLock
				}
				if err = r.api.request(recordingCtx, "GET", "/liveclientdata/allgamedata", nil, &live); err != nil {
					return result, err
				}
				_, currentTarget, targetErr := locateTarget(live.Players, request.Target)
				if targetErr != nil || currentTarget.Team != target.Team || currentTarget.IsDead == nil || !*currentTarget.IsDead ||
					live.Clock.Time == nil || math.IsNaN(*live.Clock.Time) || math.IsInf(*live.Clock.Time, 0) || math.Abs(*live.Clock.Time-state.Current) > 2 {
					return result, fmt.Errorf("%w at %.3fs (empty selection without confirmed target death)", ErrCameraLock, state.Current)
				}
			}
		}
		if locked(render, target, request.Target) {
			if *state.Recording && !preRoll {
				lifecycleReset = follower.requiresLifecycleReset(state.Current)
				if !lifecycleReset && !cameraOffsetMatches(*render.SelectionOffset, expectedCameraOffset) {
					return result, fmt.Errorf("%w at %.3fs (between-update camera follow offset readback %+v, expected %+v)", ErrCameraLock, state.Current, *render.SelectionOffset, expectedCameraOffset)
				}
				offset, ok := follower.follow(render, state.Current, time.Now())
				if !ok {
					return result, ErrCameraLock
				}
				if !cameraOffsetMatches(offset, *render.SelectionOffset) {
					const cameraTransitionDuration = 0.1
					if length-state.Current >= cameraTransitionDuration {
						endpointTime := state.Current + cameraTransitionDuration
						sequence := map[string]any{
							"selectionOffset": []map[string]any{
								{"time": state.Current, "value": *render.SelectionOffset, "blend": "linear"},
								{"time": endpointTime, "value": offset, "blend": "linear"},
							},
							"cameraRotation": []map[string]any{
								{"time": 0, "value": baseCameraRotation, "blend": "snap"},
								{"time": length, "value": baseCameraRotation, "blend": "snap"},
							},
						}
						if err = r.api.request(recordingCtx, "POST", "/replay/sequence", sequence, nil); err != nil {
							return result, fmt.Errorf("update camera follow sequence: %w", err)
						}
						var selectionLost bool
						_, selectionLost, err = r.waitForCameraOffset(recordingCtx, target, request.Target, offset, state.Current)
						if err != nil {
							if r.config.RecoverFocus && errors.Is(err, ErrCameraLock) {
								now := time.Now()
								for len(focusRecoveries) > 0 && now.Sub(focusRecoveries[0]) > 30*time.Second {
									focusRecoveries = focusRecoveries[1:]
								}
								if len(focusRecoveries) >= 5 {
									return result, err
								}
								focusRecoveries = append(focusRecoveries, now)
								if _, err = r.recoverFocus(recordingCtx, process.pid(), r.config.SelectionKeys[index], target, request.Target, state.Current); err != nil {
									return result, err
								}
								follower.suspend()
								expectedCameraOffset = baseCameraOffset
								continue
							}
							return result, err
						}
						if selectionLost {
							follower.suspend()
							if err = wait(recordingCtx, cameraPollInterval(r.config.PollInterval)); err != nil {
								return result, err
							}
							continue
						}
						expectedCameraOffset = offset
					} else {
						// Hold the last acknowledged position near the replay end.
						expectedCameraOffset = *render.SelectionOffset
					}
				} else {
					// Keep readback as the acknowledgement when smoothing does not move
					// far enough to send a new sequence update.
					expectedCameraOffset = *render.SelectionOffset
				}
			}
		}
		if *state.Recording {
			started = true
		}
		if time.Since(lastProgressAt) >= r.config.PollInterval || !*state.Recording {
			r.emit(stage, max(state.Current-fromSeconds, 0), duration)
			lastProgressAt = time.Now()
		}
		if !*state.Recording {
			if !started && state.Current < toSeconds-0.5 && time.Now().Before(startDeadline) {
				if err = wait(recordingCtx, r.config.PollInterval); err != nil {
					return result, err
				}
				continue
			}
			if !started || state.Current < toSeconds-0.5 {
				return result, ErrRecordingIncomplete
			}
			break
		}
		if err = wait(recordingCtx, cameraPollInterval(r.config.PollInterval)); err != nil {
			return result, err
		}
	}
	attemptedRecording = false
	stage = StageFinalize
	r.emit(stage, duration, duration)
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
	observation, verifyErr := r.verifier.verify(finalizeCtx, request.OutputPath, duration, request.Width, request.Height)
	if verifyErr != nil {
		err = verifyErr
		return result, err
	}
	return Result{Path: request.OutputPath, Target: request.Target, DurationSeconds: duration, ObservedDurationSeconds: floatPointer(observation.DurationSeconds), FirstVideoPTSSeconds: floatPointer(observation.FirstVideoPTSSeconds), LastVideoEndSeconds: floatPointer(observation.LastVideoEndSeconds), StartedAt: start, FinishedAt: time.Now().UTC()}, nil
}

func floatPointer(value float64) *float64 { return &value }

// selectTarget focuses the owned replay window, selects a spectator slot using
// its configured hotkey, and returns the Replay API readback for identity
// verification. Selection itself is deliberately never written through the API.
func (r *Recorder) selectTarget(ctx context.Context, pid int, key uint16) (renderState, error) {
	if err := r.desktop.selectPlayer(ctx, pid, key); err != nil {
		return renderState{}, err
	}
	if err := wait(ctx, r.config.PollInterval); err != nil {
		return renderState{}, err
	}
	var render renderState
	if err := r.api.request(ctx, "GET", "/replay/render", nil, &render); err != nil {
		return renderState{}, err
	}
	return render, nil
}

// Winsock returns WSAECONNREFUSED (10061), whereas syscall.ECONNREFUSED
// is a synthetic Go errno on Windows. Preserve all other preflight failures.
func connectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) ||
		(runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(10061)))
}
func samePath(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }
