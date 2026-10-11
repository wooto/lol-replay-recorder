package recorder

import (
	"context"
	"errors"
	"math"
	"time"
)

// SpeedSegment records contiguous game-time coverage at an acknowledged speed.
type SpeedSegment struct {
	FromSeconds float64 `json:"from_seconds"`
	ToSeconds   float64 `json:"to_seconds"`
	Speed       float64 `json:"speed"`
}

// ExpectedMediaDuration validates a complete timeline before calculating media time.
func ExpectedMediaDuration(segments []SpeedSegment, from, to float64) (float64, error) {
	if !finiteNumber(from) || !finiteNumber(to) || from < 0 || to <= from || len(segments) == 0 {
		return 0, errors.New("invalid speed timeline bounds")
	}
	at, total := from, 0.0
	for _, s := range segments {
		if !finiteNumber(s.FromSeconds) || !finiteNumber(s.ToSeconds) || !finiteNumber(s.Speed) || s.FromSeconds != at || s.ToSeconds <= s.FromSeconds || s.ToSeconds > to || (s.Speed != 1 && s.Speed != 4) {
			return 0, errors.New("invalid speed timeline coverage")
		}
		total += (s.ToSeconds - s.FromSeconds) / s.Speed
		at = s.ToSeconds
	}
	if at != to {
		return 0, errors.New("incomplete speed timeline")
	}
	return total, nil
}

type deathSpeedPolicy struct {
	since    float64
	tracking bool
	finished bool
}

func (p *deathSpeedPolicy) desired(at float64, dead *bool, remaining *float64, valid bool) float64 {
	if !valid || dead == nil {
		p.tracking = false
		p.finished = false
		return 1
	}
	if !*dead {
		p.tracking = false
		p.finished = false
		return 1
	}
	if !p.tracking {
		p.since = at
		p.tracking = true
	}
	if remaining == nil || !finiteNumber(*remaining) || *remaining <= 0 {
		return 1
	}
	if *remaining <= 3 {
		p.finished = true
	}
	if p.finished || at-p.since < 5 {
		return 1
	}
	return 4
}

type speedTimeline struct {
	from, to, at, speed float64
	segments            []SpeedSegment
	policy              deathSpeedPolicy
}

func (t *speedTimeline) change(at, speed float64) {
	at = math.Max(t.from, math.Min(t.to, at))
	if speed == t.speed {
		return
	}
	if at > t.at {
		t.segments = append(t.segments, SpeedSegment{t.at, at, t.speed})
	}
	t.at = at
	t.speed = speed
}
func (t *speedTimeline) finish() []SpeedSegment {
	if t.at < t.to {
		t.segments = append(t.segments, SpeedSegment{t.at, t.to, t.speed})
		t.at = t.to
	}
	return t.segments
}

// This runs in the existing recorder loop; it never competes with camera writes.
func (r *Recorder) updateAutoSpeed(ctx context.Context, pid int, target player, id RiotID, t *speedTimeline) error {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	var owner gameState
	if err := r.api.request(ctx, "GET", "/replay/game", nil, &owner); err != nil {
		return err
	}
	if owner.PID != pid {
		return errors.New("speed controller ownership changed")
	}
	var playback playbackState
	if err := r.api.request(ctx, "GET", "/replay/playback", nil, &playback); err != nil {
		return err
	}
	if !finiteNumber(playback.Time) || playback.Seeking || playback.Paused || playback.Time < t.at || math.Abs(playback.Speed-t.speed) > 0.01 || !finitePositive(playback.Speed) {
		return errors.New("unexpected playback during automatic speed control")
	}
	var live gameData
	err := r.api.request(ctx, "GET", "/liveclientdata/allgamedata", nil, &live)
	if err := r.api.request(ctx, "GET", "/replay/playback", nil, &playback); err != nil {
		return err
	}
	_, current, locateErr := locateTarget(live.Players, id)
	valid := err == nil && locateErr == nil && current.Team == target.Team && live.Clock.Time != nil && finiteNumber(*live.Clock.Time) && math.Abs(*live.Clock.Time-playback.Time) <= 2
	// Reserve the whole bounded controller budget and the next poll at 4x.
	// Restoration therefore starts early enough to retain three normal seconds.
	margin := 4 * (1.0 + cameraPollInterval(r.config.PollInterval).Seconds())
	remaining := current.RespawnTimer
	if valid && remaining != nil {
		projected := *remaining - math.Max(0, playback.Time-*live.Clock.Time) - margin
		remaining = &projected
	}
	want := t.policy.desired(playback.Time, current.IsDead, remaining, valid)
	if playback.Time >= t.to-3-margin {
		want = 1
	}
	if want == t.speed {
		return nil
	}
	return r.applyAutoSpeed(ctx, pid, t, want)
}

func speedCycleContext(ctx context.Context, speed float64) (context.Context, context.CancelFunc) {
	if speed == 4 {
		return context.WithTimeout(ctx, 500*time.Millisecond)
	}
	return ctx, func() {}
}

func (r *Recorder) applyAutoSpeed(ctx context.Context, pid int, t *speedTimeline, want float64) error {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	var owner gameState
	// Recheck ownership immediately before mutation, including after slow live reads.
	if err := r.api.request(ctx, "GET", "/replay/game", nil, &owner); err != nil {
		return err
	}
	if owner.PID != pid {
		return errors.New("speed controller ownership changed")
	}
	// POST returns the authoritative playback state at application, avoiding
	// an accumulated timing bias from subsequent readback latency.
	var applied playbackState
	if err := r.api.request(ctx, "POST", "/replay/playback", map[string]any{"speed": want}, &applied); err != nil {
		return err
	}
	if !finiteNumber(applied.Time) || applied.Time < t.at || !finiteNumber(applied.Speed) || math.Abs(applied.Speed-want) > 0.01 {
		return errors.New("speed application omitted authoritative state")
	}
	var playback playbackState
	if err := r.api.request(ctx, "GET", "/replay/playback", nil, &playback); err != nil {
		return err
	}
	if !finiteNumber(playback.Time) || playback.Time < t.at || !finiteNumber(playback.Speed) || math.Abs(playback.Speed-want) > 0.01 {
		return errors.New("automatic speed readback failed")
	}
	if err := r.api.request(ctx, "GET", "/replay/game", nil, &owner); err != nil {
		return err
	}
	if owner.PID != pid {
		return errors.New("speed controller ownership changed during readback")
	}
	t.change(applied.Time, want)
	r.config.Logger.Info("replay speed changed", "game_seconds", applied.Time, "speed", want)
	return nil
}
