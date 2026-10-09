package recorder

import (
	"fmt"
	"math"
	"time"
)

var (
	baseCameraOffset   = cameraVector{X: 0, Y: 1492.267578125, Z: -1006.5472412109375}
	baseCameraRotation = cameraVector{X: 0, Y: 56, Z: 0}
)

const (
	cameraFollowTimeConstant = 180 * time.Millisecond
	cameraOffsetAckInterval  = 50 * time.Millisecond
	cameraOffsetAckAttempts  = 6
	cameraOffsetAckTimeout   = 250 * time.Millisecond
	maxCameraOffsetDrift     = 500.0
	maxCameraTargetJump      = 500.0
)

func finiteCameraVector(v cameraVector) bool {
	return !math.IsNaN(v.X) && !math.IsInf(v.X, 0) &&
		!math.IsNaN(v.Y) && !math.IsInf(v.Y, 0) &&
		!math.IsNaN(v.Z) && !math.IsInf(v.Z, 0)
}

type cameraFollower struct {
	camera       cameraVector
	lastTarget   cameraVector
	lastGameTime float64
	lastUpdate   time.Time
	initialized  bool
}

func (f *cameraFollower) suspend() {
	f.initialized = false
}

func (f *cameraFollower) needsReset(state renderState, gameTime float64) bool {
	target, ok := cameraTargetPosition(state)
	return !ok || f.requiresLifecycleReset(gameTime) || horizontalDistance(target, f.lastTarget) > maxCameraTargetJump
}

func (f *cameraFollower) requiresLifecycleReset(gameTime float64) bool {
	return !f.initialized || gameTime < f.lastGameTime-0.25
}

func cameraTargetPosition(state renderState) (cameraVector, bool) {
	if state.CameraPosition == nil || state.SelectionOffset == nil {
		return cameraVector{}, false
	}
	// Replay API cameraPosition is the selected target's world position plus
	// selectionOffset, so subtract it to recover the target location.
	target := cameraVector{
		X: state.CameraPosition.X - state.SelectionOffset.X,
		Y: state.CameraPosition.Y - state.SelectionOffset.Y,
		Z: state.CameraPosition.Z - state.SelectionOffset.Z,
	}
	return target, finiteCameraVector(target)
}

func (f *cameraFollower) follow(state renderState, gameTime float64, now time.Time) (cameraVector, bool) {
	target, ok := cameraTargetPosition(state)
	if !ok {
		return cameraVector{}, false
	}
	if f.needsReset(state, gameTime) {
		f.camera = cameraVector{
			X: target.X + baseCameraOffset.X,
			Y: target.Y + baseCameraOffset.Y,
			Z: target.Z + baseCameraOffset.Z,
		}
		f.lastTarget, f.lastGameTime, f.lastUpdate, f.initialized = target, gameTime, now, true
		return baseCameraOffset, true
	}

	dt := now.Sub(f.lastUpdate).Seconds()
	if dt <= 0 {
		dt = 0.001
	}
	alpha := 1 - math.Exp(-dt/cameraFollowTimeConstant.Seconds())
	desiredX, desiredZ := target.X+baseCameraOffset.X, target.Z+baseCameraOffset.Z
	f.camera.X += (desiredX - f.camera.X) * alpha
	f.camera.Y = target.Y + baseCameraOffset.Y
	f.camera.Z += (desiredZ - f.camera.Z) * alpha
	offset := cameraVector{X: f.camera.X - target.X, Y: baseCameraOffset.Y, Z: f.camera.Z - target.Z}
	if !finiteCameraVector(offset) || horizontalDistance(offset, baseCameraOffset) > maxCameraOffsetDrift {
		f.camera = cameraVector{X: desiredX, Y: f.camera.Y, Z: desiredZ}
		offset = baseCameraOffset
	}
	f.lastTarget, f.lastGameTime, f.lastUpdate = target, gameTime, now
	return offset, true
}

func horizontalDistance(a, b cameraVector) float64 {
	return math.Hypot(a.X-b.X, a.Z-b.Z)
}

func cameraPollInterval(configured time.Duration) time.Duration {
	if configured < 50*time.Millisecond {
		return configured
	}
	return 50 * time.Millisecond
}

func cameraOffsetMatches(actual, expected cameraVector) bool {
	return horizontalDistance(actual, expected) <= 2 && math.Abs(actual.Y-expected.Y) <= 0.5
}

func cameraOffsetAckError(gameTime float64, actual *cameraVector, expected cameraVector) error {
	if actual == nil {
		return fmt.Errorf("%w at %.3fs (offset acknowledgement readback unavailable, expected %+v)", ErrCameraLock, gameTime, expected)
	}
	return fmt.Errorf("%w at %.3fs (offset acknowledgement readback %+v, expected %+v)", ErrCameraLock, gameTime, *actual, expected)
}
