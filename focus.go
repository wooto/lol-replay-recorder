package recorder

import (
	"context"
	"errors"
	"fmt"
)

func focusDrift(state renderState, target player, id RiotID, expected cameraVector, checkOffset bool) bool {
	return !locked(state, target, id) || !cameraPoseValid(state) || !cameraInputControlsValid(state) || !cameraOffsetWithinRange(state) ||
		(checkOffset && state.SelectionOffset != nil && !cameraOffsetMatches(*state.SelectionOffset, expected))
}

// Native selection performs two key taps and proves foreground window ownership.
// API writes only restore camera controls; player identity is always selected by
// its roster slot hotkey, never by writing selectionName.
func (r *Recorder) recoverFocus(ctx context.Context, pid int, key uint16, target player, id RiotID, at float64) (renderState, error) {
	var state renderState
	for attempt := 0; attempt < 3; attempt++ {
		var owner gameState
		if err := r.api.request(ctx, "GET", "/replay/game", nil, &owner); err != nil {
			return state, err
		}
		if owner.PID != pid {
			return state, errors.New("focus recovery game process changed")
		}
		var err error
		state, err = r.selectTarget(ctx, pid, key)
		if err != nil {
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
