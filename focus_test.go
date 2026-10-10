package recorder

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNativeFollowLeavesMovingAttachedCameraToGame(t *testing.T) {
	r, request, f := testRecorder(t, "camera-follow")
	r.config.NativeFollow = true
	r.config.RecoverFocus = true
	r.config.LaunchTimeout = 5 * time.Second
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(f.followOffsets) != 0 || f.selectCalls != 1 || !f.verified {
		t.Fatalf("healthy native follow was overwritten: offsets=%d selections=%d verified=%t", len(f.followOffsets), f.selectCalls, f.verified)
	}
}

func TestNativeFollowStillRecoversLostAttachment(t *testing.T) {
	r, request, f := testRecorder(t, "watch-detached")
	r.config.NativeFollow = true
	r.config.RecoverFocus = true
	r.config.LaunchTimeout = 5 * time.Second
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !f.restored || f.selectCalls < 2 || len(f.followOffsets) != 0 || !f.verified {
		t.Fatalf("native follow recovery failed: restored=%t selections=%d offsets=%d", f.restored, f.selectCalls, len(f.followOffsets))
	}
}

func TestFocusWatcherRecoversDuringRecording(t *testing.T) {
	for _, mode := range []string{"watch-wrong-player", "watch-empty", "watch-detached", "watch-position", "watch-ack", "watch-repeated"} {
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

func TestFocusWatcherDoesNotRefocusAfterCompletion(t *testing.T) {
	r, request, f := testRecorder(t, "watch-ended")
	r.config.RecoverFocus = true
	r.config.LaunchTimeout = 5 * time.Second
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if f.selectCalls != 1 || !f.verified {
		t.Fatalf("refocused after capture ended: calls=%d", f.selectCalls)
	}
}

func TestFocusWatcherStopsPersistentFlapping(t *testing.T) {
	r, request, f := testRecorder(t, "watch-flapping")
	r.config.RecoverFocus = true
	r.config.LaunchTimeout = 5 * time.Second
	_, err := r.RecordFull(context.Background(), request)
	if !errors.Is(err, ErrCameraLock) || !strings.Contains(err.Error(), "five recoveries") {
		t.Fatalf("got %v", err)
	}
	if f.selectCalls != 6 || !f.stopped || !f.closed {
		t.Fatalf("unbounded or incomplete cleanup: calls=%d stopped=%t closed=%t", f.selectCalls, f.stopped, f.closed)
	}
}

type temporarilyDeniedDesktop struct {
	desktop
	denied int
	calls  int
}

func (d *temporarilyDeniedDesktop) selectPlayer(ctx context.Context, pid int, key uint16) error {
	d.calls++
	if d.denied > 0 {
		d.denied--
		return ErrForegroundDenied
	}
	return d.desktop.selectPlayer(ctx, pid, key)
}

func TestFocusRecoveryRetriesTransientForegroundDenial(t *testing.T) {
	r, request, f := testRecorder(t, "")
	f.launched = true
	d := &temporarilyDeniedDesktop{desktop: r.desktop, denied: 1}
	r.desktop = d
	if _, err := r.recoverFocus(context.Background(), 4242, '2', player{SummonerName: "Player#KR1"}, request.Target, 60); err != nil {
		t.Fatal(err)
	}
	if d.calls != 2 || f.selectCalls != 1 {
		t.Fatalf("calls=%d delivered=%d", d.calls, f.selectCalls)
	}
}

func TestFocusRecoveryCancellationDoesNotPressKeys(t *testing.T) {
	r, request, f := testRecorder(t, "")
	f.launched = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.recoverFocus(ctx, 4242, '2', player{}, request.Target, 60); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if f.selectCalls != 0 || f.cameraControlsSet {
		t.Fatal("acted after cancellation")
	}
}

func TestFocusRecoveryBothTeamSlots(t *testing.T) {
	for _, key := range []uint16{'1', '2', '3', '4', '5', 'Q', 'W', 'E', 'R', 'T'} {
		t.Run(string(rune(key)), func(t *testing.T) {
			r, request, f := testRecorder(t, "")
			f.launched = true
			f.expectedSelectionKey = key
			if _, err := r.recoverFocus(context.Background(), 4242, key, player{SummonerName: "Player#KR1"}, request.Target, 60); err != nil {
				t.Fatal(err)
			}
			if f.selectCalls != 1 || f.selectionWrites != 0 {
				t.Fatal("did not use the roster slot hotkey")
			}
		})
	}
}

func TestFocusBudgetSharedSlidingWindow(t *testing.T) {
	var b focusBudget
	start := time.Unix(1000, 0)
	for i := 0; i < 5; i++ {
		if !b.allow(start.Add(time.Duration(i) * time.Second)) {
			t.Fatal("blocked within budget")
		}
	}
	if b.allow(start.Add(30 * time.Second)) {
		t.Fatal("allowed sixth recovery before window expiry")
	}
	if !b.allow(start.Add(31 * time.Second)) {
		t.Fatal("did not replenish expired budget")
	}
}

func TestFocusRecoveryRechecksOwnerAfterHotkey(t *testing.T) {
	r, request, f := testRecorder(t, "watch-owner")
	f.launched = true
	_, err := r.recoverFocus(context.Background(), 4242, '2', player{SummonerName: "Player#KR1"}, request.Target, 60)
	if err == nil || !strings.Contains(err.Error(), "process changed") {
		t.Fatalf("accepted new replay owner: %v", err)
	}
	if f.cameraControlsSet {
		t.Fatal("wrote camera controls after ownership changed")
	}
}

func TestFocusWatcherDeathRespawn(t *testing.T) {
	for _, mode := range []string{"death-respawn", "respawn-race", "camera-follow-death-ack"} {
		t.Run(mode, func(t *testing.T) {
			r, request, f := testRecorder(t, mode)
			r.config.RecoverFocus = true
			r.config.LaunchTimeout = 5 * time.Second
			if _, err := r.RecordFull(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if !f.verified || !f.closed {
				t.Fatal("death/respawn did not finish validation")
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
