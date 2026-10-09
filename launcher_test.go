package recorder

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFullRecordingKeepsCameraAboveTargetAcrossEncoderSeeks(t *testing.T) {
	r, request, _ := testRecorder(t, "camera-offset")
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("selected player must retain an elevated camera and normal angle through the full recording: %v", err)
	}
}

func TestFullRecordingNaturallyFollowsMovingTarget(t *testing.T) {
	r, request, f := testRecorder(t, "camera-follow")
	r.config.PollInterval = 50 * time.Millisecond
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("moving target must complete a full recording: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	baseZ := -1006.5472412109375
	lagged := false
	for _, offset := range f.followOffsets {
		if offset.Y != 1492.267578125 {
			t.Fatalf("camera follow changed the target-relative height: %+v", offset)
		}
		if offset.X != 0 || offset.Z != baseZ {
			lagged = true
		}
	}
	if !lagged {
		t.Fatal("camera stayed rigidly attached instead of easing behind the moving target")
	}
	if !f.cameraTrack || !f.cameraOffsetTrack {
		t.Fatal("camera sequence must keep selection and rotation while updating its dynamic offset track")
	}
}

func TestCameraFollowSequenceWorksWhenRenderOffsetUpdatesAreIgnored(t *testing.T) {
	r, request, f := testRecorder(t, "camera-follow-ignored-offset")
	r.config.PollInterval = 50 * time.Millisecond
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("dynamic camera follow sequence must work when render offset POSTs are ignored: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cameraTrack || !f.cameraOffsetTrack || len(f.followOffsets) == 0 {
		t.Fatal("camera follow did not update the selection-offset sequence while preserving target and rotation tracks")
	}
}

func TestIgnoredCameraFollowSequenceCannotReportFullSuccess(t *testing.T) {
	r, request, f := testRecorder(t, "camera-follow-sequence-ignored")
	r.config.PollInterval = 50 * time.Millisecond
	if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
		t.Fatalf("ignored camera follow sequence must fail readback verification: %v", err)
	}
	if !f.closed {
		t.Fatal("game was left running after ignored camera follow sequence")
	}
}

func TestDelayedCameraFollowOffsetEchoIsAcknowledged(t *testing.T) {
	r, request, f := testRecorder(t, "camera-follow-delayed-offset")
	r.config.PollInterval = 50 * time.Millisecond
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("delayed but applied camera follow update must complete: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.followOffsets) == 0 {
		t.Fatal("delayed camera follow update was never applied")
	}
}

func TestDeathDuringCameraOffsetAcknowledgementUsesBoundedRecovery(t *testing.T) {
	r, request, f := testRecorder(t, "camera-follow-death-ack")
	r.config.PollInterval = 50 * time.Millisecond
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("confirmed death during camera offset acknowledgement must recover: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.deathAckPostIgnored || !f.deathAckLockedRead || !f.deathAckStaleOffset || f.deathAckRenderReads != 2 || !f.deathAckEmptySeen || !f.deathAckDeathSeen || f.deathAckDeathClock != 60 || !f.deathAckReacquired || !f.verified {
		t.Fatalf("death during offset acknowledgement did not use the bounded death/reacquisition path: ignoredPOST=%t lockedRead=%t staleOffset=%t reads=%d empty=%t death=%t liveClock=%v reacquired=%t verified=%t",
			f.deathAckPostIgnored, f.deathAckLockedRead, f.deathAckStaleOffset, f.deathAckRenderReads, f.deathAckEmptySeen, f.deathAckDeathSeen, f.deathAckDeathClock, f.deathAckReacquired, f.verified)
	}
}

func TestUncommandedCameraOffsetDriftCannotResetFollower(t *testing.T) {
	for _, mode := range []string{"camera-offset-drift-bounded", "camera-offset-drift-large"} {
		t.Run(mode, func(t *testing.T) {
			r, request, _ := testRecorder(t, mode)
			r.config.PollInterval = 50 * time.Millisecond
			if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
				t.Fatalf("uncommanded camera offset drift must fail: %v", err)
			}
		})
	}
}

func TestAcceptedButIgnoredCameraProfileDoesNotStartRecording(t *testing.T) {
	r, request, f := testRecorder(t, "camera-profile-ignored")
	if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
		t.Fatalf("unverified camera position must fail: %v", err)
	}
	if f.path != "" || !f.closed {
		t.Fatal("unverified camera started recording or left game running")
	}
}

func TestStaleBoundedCameraOffsetCannotStartRecording(t *testing.T) {
	r, request, f := testRecorder(t, "camera-profile-offset-ignored")
	if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
		t.Fatalf("stale bounded camera offset must fail preflight: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.path != "" || !f.closed {
		t.Fatal("stale camera offset started recording or left the game running")
	}
}

func TestIgnoredCameraInputControlsCannotStartRecording(t *testing.T) {
	r, request, f := testRecorder(t, "camera-input-controls-ignored")
	if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
		t.Fatalf("unverified unlocked FPS input controls must fail before recording: %v", err)
	}
	if f.path != "" || !f.closed {
		t.Fatal("recording started or the owned game was left running with unverified FPS input controls")
	}
}

func TestCameraInputControlDriftDuringRecordingCannotReportFullSuccess(t *testing.T) {
	r, request, _ := testRecorder(t, "camera-input-controls-drift")
	if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
		t.Fatalf("FPS input-control drift during capture must fail: %v", err)
	}
}

func TestCameraOffsetLossDuringRecordingCannotReportFullSuccess(t *testing.T) {
	r, request, _ := testRecorder(t, "camera-profile-drift")
	if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
		t.Fatalf("camera inside terrain must not report FULL: %v", err)
	}
}

func TestMissingCameraPositionCannotRecord(t *testing.T) {
	r, request, f := testRecorder(t, "camera-position-missing")
	if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
		t.Fatalf("camera without a world position must fail: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.path != "" || !f.closed {
		t.Fatal("missing camera position started recording or left the game running")
	}
}

func TestAudioDurationCannotHideShortVideo(t *testing.T) {
	r, request, _ := testRecorder(t, "native-video-short")
	if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrRecordingIncomplete) {
		t.Fatalf("full audio/container duration cannot prove full video: %v", err)
	}
}

func TestNativeCameraPrerollPreservesTimeZeroThroughVideoEnd(t *testing.T) {
	r, request, _ := testRecorder(t, "native-preroll")
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("native capture must retain time zero and the final video frame: %v", err)
	}
}

func TestNativeFFprobeEmptyProgramSectionDoesNotRejectFullVideo(t *testing.T) {
	r, request, _ := testRecorder(t, "native-empty-programs")
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatal(err)
	}
}

type exportedFakeProcess struct{ fakeProcess }

func (p exportedFakeProcess) PID() int     { return p.pid() }
func (p exportedFakeProcess) Exited() bool { return p.exited() }
func (p exportedFakeProcess) Close() error { return p.close() }

func TestCustomLauncherUsesOwnedProcessAndSelectionSequence(t *testing.T) {
	r, request, f := testRecorder(t, "api-selection")
	called := false
	r.config.LaunchReplay = func(ctx context.Context, path string) (ReplayProcess, error) {
		if path != request.ReplayPath {
			t.Fatalf("unexpected replay: %s", path)
		}
		called = true
		f.mu.Lock()
		f.launched = true
		f.mu.Unlock()
		return exportedFakeProcess{fakeProcess{f}}, nil
	}
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !called || !f.closed || !f.verified {
		t.Fatal("custom process was not recorded, verified, and closed")
	}
	if len(f.sequence) != 2 || f.sequence[0].Time != 0 || f.sequence[1].Time != 90 || f.sequence[0].Value != "Player#KR1" || f.sequence[1].Value != "Player#KR1" {
		t.Fatalf("missing constant verified selection: %+v", f.sequence)
	}
}

func TestSequenceFailurePreventsRecording(t *testing.T) {
	r, request, f := testRecorder(t, "sequence-error")
	if _, err := r.RecordFull(context.Background(), request); err == nil {
		t.Fatal("sequence error must fail")
	}
	if f.path != "" || !f.closed {
		t.Fatal("sequence failure recorded or left game running")
	}
}

func TestNilCustomProcessRejected(t *testing.T) {
	r, request, _ := testRecorder(t, "")
	r.config.LaunchReplay = func(context.Context, string) (ReplayProcess, error) { return nil, nil }
	if _, err := r.RecordFull(context.Background(), request); err == nil {
		t.Fatal("nil custom process must fail")
	}
}

func TestSelectionFallsBackWhenRenderAPIAcceptsButIgnoresName(t *testing.T) {
	r, request, _ := testRecorder(t, "ignored-api")
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("verified keyboard fallback should record even when name POST is ignored: %v", err)
	}
}

func TestFullRecordingIncludesDeathAndRespawnWithoutSelectingAnotherPlayer(t *testing.T) {
	r, request, _ := testRecorder(t, "death-respawn")
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("confirmed target death should retain the selection track until respawn: %v", err)
	}
}

func TestDeathNeverPermitsAnotherCameraOrUnconfirmedTargetState(t *testing.T) {
	for _, mode := range []string{"death-unattached", "death-other-player", "death-unknown"} {
		t.Run(mode, func(t *testing.T) {
			r, request, _ := testRecorder(t, mode)
			if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
				t.Fatalf("unproven camera must fail: %v", err)
			}
		})
	}
}

func TestFullRecordingReverifiesEmptySelectionDuringRespawn(t *testing.T) {
	r, request, _ := testRecorder(t, "respawn-race")
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("the same target successfully reselected must resume: %v", err)
	}
}

func TestStaleDeathSnapshotCannotExcuseCameraLoss(t *testing.T) {
	r, request, _ := testRecorder(t, "death-stale-data")
	if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
		t.Fatalf("death from another game time cannot prove current camera state: %v", err)
	}
}

func TestFullRecordingValidatesOutputWhenEncoderClearsCompletedStatus(t *testing.T) {
	r, request, f := testRecorder(t, "reset-on-complete")
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("completed recording must reach output verification: %v", err)
	}
	if !f.verified {
		t.Fatal("reset completion skipped output verification")
	}
}

func TestRecordingDoesNotOverrideEncoderPlaybackClock(t *testing.T) {
	r, request, _ := testRecorder(t, "encoder-clock")
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("encoder must retain its chosen playback timing: %v", err)
	}
}

func TestNativeCompletionRetainsPathButClearsEndMarker(t *testing.T) {
	r, request, _ := testRecorder(t, "native-complete")
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("native completion must verify full output: %v", err)
	}
}

func TestClearedCompletionStillRequiresTargetCamera(t *testing.T) {
	r, request, _ := testRecorder(t, "reset-lost-lock")
	if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
		t.Fatalf("completion cannot bypass target camera proof: %v", err)
	}
}

func TestNativeRecordingPreservesMatchTimeInOutputVideo(t *testing.T) {
	r, request, _ := testRecorder(t, "native-wall-clock")
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("full video must retain normal match duration: %v", err)
	}
}
