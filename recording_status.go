package recorder

import (
	"context"
	"errors"
	"net"
	"time"
)

// Return to the ownership/recording checks after a stalled ending read. The
// caller bounds the entire ending phase and never reuses the stale camera frame.
func retryEndingRead(ctx context.Context, err error, nearEnd bool) bool {
	var timeout net.Error
	return nearEnd && ctx.Err() == nil && errors.As(err, &timeout) && timeout.Timeout() && wait(ctx, 250*time.Millisecond) == nil
}

// Long captures can block the status endpoint while flushing their encoder.
// Retry reads only near the already observed end, within the finalization bound.
// A recovered response must still belong to the same process; normal range and
// output validation remains the caller's responsibility.
func (r *Recorder) recordingStatus(ctx context.Context, pid int, nearEnd bool) (recordingState, error) {
	readCtx, cancel := context.WithTimeout(ctx, r.config.FinalizeTimeout)
	defer cancel()
	retried := false
	for {
		var state recordingState
		err := r.api.request(readCtx, "GET", "/replay/recording", nil, &state)
		if err == nil {
			if retried {
				return state, r.focusOwner(readCtx, pid)
			}
			return state, nil
		}
		var timeout net.Error
		if !nearEnd || readCtx.Err() != nil || !errors.As(err, &timeout) || !timeout.Timeout() {
			return state, err
		}
		if err = wait(readCtx, 250*time.Millisecond); err != nil {
			return state, err
		}
		retried = true
	}
}
