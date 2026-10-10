package recorder

import (
	"context"
	"errors"
	"net"
	"time"
)

// Long captures can block the status endpoint while flushing their encoder.
// Retry reads only near the already observed end, within the finalization bound.
// A recovered response must still belong to the same process; normal range and
// output validation remains the caller's responsibility.
func (r *Recorder) recordingStatus(ctx context.Context, pid int, nearEnd bool) (recordingState, error) {
	readCtx, cancel := context.WithTimeout(ctx, r.config.FinalizeTimeout)
	defer cancel()
	for {
		var state recordingState
		err := r.api.request(readCtx, "GET", "/replay/recording", nil, &state)
		if err == nil {
			return state, r.focusOwner(readCtx, pid)
		}
		var timeout net.Error
		if !nearEnd || readCtx.Err() != nil || !errors.As(err, &timeout) || !timeout.Timeout() {
			return state, err
		}
		if err = wait(readCtx, 250*time.Millisecond); err != nil {
			return state, err
		}
	}
}
