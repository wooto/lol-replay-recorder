package recorder

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecordingStatusFinalizationTimeout(t *testing.T) {
	for _, mode := range []string{"near-end", "mid-capture", "changed-owner", "http-error", "persistent-timeout"} {
		t.Run(mode, func(t *testing.T) {
			var reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Method != "GET" {
					t.Error("retried a mutation")
				}
				if req.URL.Path == "/replay/game" {
					pid := 4242
					if mode == "changed-owner" {
						pid++
					}
					fmt.Fprintf(w, `{"processID":%d}`, pid)
					return
				}
				n := reads.Add(1)
				if mode == "http-error" {
					http.Error(w, "failed", 500)
					return
				}
				if n == 1 || mode == "persistent-timeout" {
					time.Sleep(50 * time.Millisecond)
				}
				fmt.Fprint(w, `{"recording":false,"currentTime":90}`)
			}))
			defer server.Close()
			api, err := newAPI(server.URL, 20*time.Millisecond, false)
			if err != nil {
				t.Fatal(err)
			}
			defer api.close()
			r := &Recorder{api: api, config: Config{FinalizeTimeout: time.Second}}
			if mode == "persistent-timeout" {
				r.config.FinalizeTimeout = 100 * time.Millisecond
			}
			state, err := r.recordingStatus(context.Background(), 4242, mode != "mid-capture")
			if mode == "near-end" {
				if err != nil || state.Recording == nil || *state.Recording || reads.Load() != 2 {
					t.Fatalf("state=%+v err=%v reads=%d", state, err, reads.Load())
				}
			} else if err == nil {
				t.Fatal("accepted failed status")
			}
			if mode == "changed-owner" && !strings.Contains(err.Error(), "process changed") {
				t.Fatal(err)
			}
			if (mode == "http-error" || mode == "mid-capture") && reads.Load() != 1 {
				t.Fatalf("unexpected retries: %d", reads.Load())
			}
		})
	}
}

func TestRecordingCompletesAfterEndingCameraOrOwnerTimeout(t *testing.T) {
	for _, mode := range []string{"ending-render-timeout", "ending-owner-timeout", "ending-render-reset"} {
		t.Run(mode, func(t *testing.T) {
			r, request, f := testRecorder(t, mode)
			r.config.RecoverFocus = true
			r.config.NativeFollow = true
			r.config.FinalizeTimeout = 2 * time.Second
			r.config.LaunchTimeout = 5 * time.Second
			r.api.client.Timeout = 20 * time.Millisecond
			if _, err := r.RecordFull(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if (!f.endingReadDelayed && mode != "ending-render-reset") || !f.verified || !f.closed || f.selectCalls != 1 {
				t.Fatalf("delayed=%t verified=%t closed=%t selections=%d", f.endingReadDelayed, f.verified, f.closed, f.selectCalls)
			}
		})
	}
}

func TestEndingReadRetryRejectsMidCaptureAndPersistentTimeouts(t *testing.T) {
	for _, mode := range []string{"mid-render-timeout", "ending-owner-persistent"} {
		t.Run(mode, func(t *testing.T) {
			r, request, f := testRecorder(t, mode)
			r.config.RecoverFocus = true
			r.config.NativeFollow = true
			r.config.LaunchTimeout = 5 * time.Second
			r.config.FinalizeTimeout = 350 * time.Millisecond
			r.api.client.Timeout = 20 * time.Millisecond
			if _, err := r.RecordFull(context.Background(), request); err == nil {
				t.Fatal("accepted an unverified or indefinitely stalled capture")
			}
			if !f.endingReadDelayed || f.verified || !f.closed {
				t.Fatalf("delayed=%t verified=%t closed=%t", f.endingReadDelayed, f.verified, f.closed)
			}
		})
	}
}
