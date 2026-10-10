package recorder

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

func focusDrift(state renderState, target player, id RiotID, expected cameraVector, checkOffset bool) bool {
	return !locked(state, target, id) || !cameraPoseValid(state) || state.CameraPosition == nil || *state.CameraPosition == (cameraVector{}) || !cameraInputControlsValid(state) || !cameraOffsetWithinRange(state) ||
		(checkOffset && state.SelectionOffset != nil && !cameraOffsetMatches(*state.SelectionOffset, expected))
}

type focusBudget struct{ recoveries []time.Time }

func (b *focusBudget) allow(now time.Time) bool {
	for len(b.recoveries) > 0 && now.Sub(b.recoveries[0]) > 30*time.Second {
		b.recoveries = b.recoveries[1:]
	}
	if len(b.recoveries) >= 5 {
		return false
	}
	b.recoveries = append(b.recoveries, now)
	return true
}

func (r *Recorder) focusOwner(ctx context.Context, pid int) error {
	var owner gameState
	if err := r.api.request(ctx, "GET", "/replay/game", nil, &owner); err != nil {
		return err
	}
	if pid <= 0 || owner.PID != pid {
		return errors.New("focus recovery game process changed")
	}
	return nil
}

// Native selection performs two key taps and proves foreground window ownership.
// API writes only restore camera controls; player identity is always selected by
// its roster slot hotkey, never by writing selectionName.
func (r *Recorder) recoverFocus(ctx context.Context, pid int, key uint16, target player, id RiotID, at float64) (renderState, error) {
	var state renderState
	for attempt := 0; attempt < 3; attempt++ {
		if err := r.focusOwner(ctx, pid); err != nil {
			return state, err
		}
		var err error
		state, err = r.selectTarget(ctx, pid, key)
		if err != nil {
			if errors.Is(err, ErrForegroundDenied) && attempt < 2 {
				if err = wait(ctx, 150*time.Millisecond); err != nil {
					return state, err
				}
				continue
			}
			return state, err
		}
		if err = r.focusOwner(ctx, pid); err != nil {
			return state, err
		}
		// A hotkey may select the right player without attaching the camera. Check
		// identity independently before restoring attachment and the elevated view.
		attached := true
		identity := state
		identity.CameraAttached = &attached
		if !locked(identity, target, id) {
			continue
		}
		if err = r.api.request(ctx, "POST", "/replay/sequence", map[string]any{"selectionOffset": []cameraVector{}}, nil); err != nil {
			return state, err
		}
		if err = r.focusOwner(ctx, pid); err != nil {
			return state, err
		}
		if err = r.api.request(ctx, "POST", "/replay/render", map[string]any{
			"cameraAttached": true, "cameraMode": "fps", "selectionOffset": baseCameraOffset, "cameraRotation": baseCameraRotation,
			"cameraLockX": false, "cameraLockY": false, "cameraLockZ": false, "cameraMoveSpeed": 0, "cameraLookSpeed": 0,
		}, nil); err != nil {
			return state, err
		}
		// Require multiple readbacks, rather than trusting the first transient frame.
		good := 0
		for sample := 0; sample < 6; sample++ {
			if err = wait(ctx, cameraOffsetAckInterval); err != nil {
				return state, err
			}
			if err = r.api.request(ctx, "GET", "/replay/render", nil, &state); err != nil {
				return state, err
			}
			if err = r.focusOwner(ctx, pid); err != nil {
				return state, err
			}
			if focusDrift(state, target, id, baseCameraOffset, true) {
				good = 0
			} else {
				good++
			}
			if good >= 3 {
				r.config.Logger.Info("target focus restored", "game_seconds", at, "attempt", attempt+1)
				r.emit(Stage("refocus"), at, 0)
				return state, nil
			}
		}
	}
	return state, fmt.Errorf("%w at %.3fs after three hotkey recovery attempts (selected %q)", ErrCameraLock, at, state.SelectionName)
}

// A dead champion can have no selectable render object. Restore native attachment
// only after proving this exact target's death, and require normal selection again
// as soon as live data reports it alive.
func (r *Recorder) recoverDeadFocus(ctx context.Context, pid int, key uint16, target player, id RiotID, at float64) (renderState, error) {
	if err := r.focusOwner(ctx, pid); err != nil {
		return renderState{}, err
	}
	state, err := r.selectTarget(ctx, pid, key)
	if err != nil {
		return state, err
	}
	for sample := 0; sample < 3; sample++ {
		if err = r.focusOwner(ctx, pid); err != nil {
			return state, err
		}
		var live gameData
		if err = r.api.request(ctx, "GET", "/liveclientdata/allgamedata", nil, &live); err != nil {
			return state, err
		}
		_, current, locateErr := locateTarget(live.Players, id)
		if locateErr != nil || current.Team != target.Team || current.IsDead == nil || live.Clock.Time == nil || !finiteNumber(*live.Clock.Time) || math.Abs(*live.Clock.Time-at) > 2 {
			return state, fmt.Errorf("%w during dead target recovery at %.3f (sample %d, selected %q, attached %v)", ErrCameraLock, at, sample, state.SelectionName, state.CameraAttached)
		}
		if !*current.IsDead {
			return r.recoverFocus(ctx, pid, key, target, id, at)
		}
		identity := state
		attached := true
		identity.CameraAttached = &attached
		if state.SelectionName != "" && !locked(identity, target, id) {
			return state, fmt.Errorf("%w during dead target recovery at %.3f (sample %d, selected %q, attached %v)", ErrCameraLock, at, sample, state.SelectionName, state.CameraAttached)
		}
		if sample == 0 {
			if err = r.focusOwner(ctx, pid); err != nil {
				return state, err
			}
			if err = r.api.request(ctx, "POST", "/replay/render", map[string]any{"cameraAttached": true, "cameraMode": "fps", "selectionOffset": baseCameraOffset, "cameraRotation": baseCameraRotation, "cameraLockX": false, "cameraLockY": false, "cameraLockZ": false, "cameraMoveSpeed": 0, "cameraLookSpeed": 0}, nil); err != nil {
				return state, err
			}
		}
		if err = wait(ctx, cameraOffsetAckInterval); err != nil {
			return state, err
		}
		if err = r.api.request(ctx, "GET", "/replay/render", nil, &state); err != nil {
			return state, err
		}
		if err = r.focusOwner(ctx, pid); err != nil {
			return state, err
		}
		if state.CameraAttached == nil || !cameraPoseValid(state) || !cameraInputControlsValid(state) || !cameraOffsetWithinRange(state) || state.SelectionOffset == nil || !cameraOffsetMatches(*state.SelectionOffset, baseCameraOffset) || (state.SelectionName != "" && !selectedIdentity(state, target, id)) {
			return state, fmt.Errorf("%w during dead target recovery at %.3f (sample %d, selected %q, attached %v)", ErrCameraLock, at, sample, state.SelectionName, state.CameraAttached)
		}
	}
	if state.CameraAttached != nil && *state.CameraAttached {
		r.emit(Stage("refocus"), at, 0)
	} else {
		r.emit(Stage("refocus-pending"), at, 0)
	}
	return state, nil
}
