package recorder

import (
	"context"
	"errors"
	"testing"
)

func TestFullRecordingKeepsCameraAboveTargetAcrossEncoderSeeks(t *testing.T) {
	r, request, _ := testRecorder(t, "camera-offset")
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("selected player must retain an elevated camera and normal angle through the full recording: %v", err)
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

func TestCameraOffsetLossDuringRecordingCannotReportFullSuccess(t *testing.T) {
	r, request, _ := testRecorder(t, "camera-profile-drift")
	if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
		t.Fatalf("camera inside terrain must not report FULL: %v", err)
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
