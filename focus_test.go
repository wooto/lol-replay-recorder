package recorder

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFocusWatcherRecoversDuringRecording(t *testing.T) {
	for _, mode := range []string{"watch-wrong-player", "watch-empty", "watch-detached", "watch-position", "watch-ack"} {
		t.Run(mode, func(t *testing.T) {
			r, request, f := testRecorder(t, mode)
			r.config.RecoverFocus = true
			r.config.LaunchTimeout = 5 * time.Second
			if _, err := r.RecordFull(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if !f.restored || f.selectCalls < 2 || !f.verified {
				t.Fatalf("watcher did not recover and verify: restored=%t calls=%d", f.restored, f.selectCalls)
			}
			if f.selectionWrites != 0 {
				t.Fatal("selected identity through API")
			}
		})
	}
}

func TestFocusWatcherStopsWhenRecoveryIsIgnored(t *testing.T) {
	r, request, f := testRecorder(t, "lost-lock")
	r.config.RecoverFocus = true
	r.config.LaunchTimeout = 5 * time.Second
	if _, err := r.RecordFull(context.Background(), request); !errors.Is(err, ErrCameraLock) {
		t.Fatalf("got %v", err)
	}
	if f.selectCalls != 4 || !f.stopped || !f.closed {
		t.Fatalf("retries/cleanup: calls=%d stopped=%t closed=%t", f.selectCalls, f.stopped, f.closed)
	}
}
