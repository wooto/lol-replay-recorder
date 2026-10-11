package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDeathSpeedPolicy(t *testing.T) {
	p := deathSpeedPolicy{}
	dead, alive := true, false
	timer := 20.0
	for _, v := range []struct {
		at    float64
		dead  *bool
		timer *float64
		valid bool
		want  float64
	}{
		{100, &dead, &timer, true, 1}, {104.9, &dead, &timer, true, 1}, {105, &dead, &timer, true, 4},
		{106, &dead, nil, true, 1}, {107, &dead, &timer, true, 4}, {108, &dead, &timer, false, 1},
		{109, &dead, &timer, true, 1}, {114, &dead, &timer, true, 4}, {115, &alive, &timer, true, 1},
	} {
		if got := p.desired(v.at, v.dead, v.timer, v.valid); got != v.want {
			t.Fatalf("at %v got %v want %v", v.at, got, v.want)
		}
	}
	p = deathSpeedPolicy{}
	p.desired(0, &dead, &timer, true)
	timer = 3
	if p.desired(6, &dead, &timer, true) != 1 {
		t.Fatal("respawn boundary accelerated")
	}
	timer = 4
	if p.desired(7, &dead, &timer, true) != 1 {
		t.Fatal("timer jitter reentered speed")
	}
	for _, invalid := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if p.desired(10, &dead, &invalid, true) != 1 {
			t.Fatal("invalid timer accelerated")
		}
	}
}

func TestAutoSpeedMainLoopBoundsSlowRender(t *testing.T) {
	r, request, f := testRecorder(t, "ok")
	speed := 1.0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		f.mu.Lock()
		active, at := f.path != "", float64(f.ticks)*30
		paused := !active
		f.mu.Unlock()
		if q.URL.Path == "/replay/playback" {
			if q.Method == "POST" {
				raw, _ := io.ReadAll(q.Body)
				var body struct {
					Speed *float64 `json:"speed"`
				}
				json.Unmarshal(raw, &body)
				if body.Speed != nil {
					speed = *body.Speed
				}
				q.Body = io.NopCloser(bytes.NewReader(raw))
				f.serve(httptest.NewRecorder(), q)
			}
			f.mu.Lock()
			at = math.Max(at, f.playbackTime)
			f.mu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{"length": 90, "time": at, "speed": speed, "paused": paused})
			return
		}
		if active && q.URL.Path == "/liveclientdata/allgamedata" {
			json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{"gameTime": at}, "allPlayers": []any{map[string]any{"riotId": "Player#KR1", "team": "ORDER", "isDead": true, "respawnTimer": 30}}})
			return
		}
		if speed == 4 && q.URL.Path == "/replay/render" && q.Method == "GET" {
			select {
			case <-q.Context().Done():
				return
			case <-time.After(time.Second):
			}
		}
		f.serve(w, q)
	}))
	defer server.Close()
	api, err := newAPI(server.URL, 2*time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	defer api.close()
	r.api = api
	r.config.AutoSpeed = true
	r.config.RecoverFocus = true
	r.config.NativeFollow = true
	r.config.LaunchTimeout = 5 * time.Second
	_, err = r.RecordFull(context.Background(), request)
	if !errors.Is(err, context.DeadlineExceeded) || f.verified || !f.closed || speed != 4 {
		t.Fatalf("slow accelerated main loop accepted: %v verified=%v closed=%v speed=%v", err, f.verified, f.closed, speed)
	}
}

func TestSpeedApplicationUsesPostTimestamp(t *testing.T) {
	at, speed := 70.0, 1.0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		out := any(map[string]any{"processID": 42})
		if q.URL.Path == "/replay/playback" {
			if q.Method == "POST" {
				var body struct {
					Speed float64 `json:"speed"`
				}
				json.NewDecoder(q.Body).Decode(&body)
				speed = body.Speed
				at += 0.1
				json.NewEncoder(w).Encode(map[string]any{"time": at, "speed": speed})
				at += 0.4
				return
			}
			out = map[string]any{"time": at, "speed": speed}
		}
		json.NewEncoder(w).Encode(out)
	}))
	defer server.Close()
	r, err := New(Config{ReplayURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	x := speedTimeline{from: 60, to: 90, at: 60, speed: 1}
	for _, want := range []float64{4, 1, 4, 1} {
		before := at
		if err := r.applyAutoSpeed(context.Background(), 42, &x, want); err != nil {
			t.Fatal(err)
		}
		if x.at != before+0.1 {
			t.Fatalf("readback latency polluted transition %v", x.at)
		}
	}
}

func TestSpeedRestoresBeforeRespawnWithDelayedSample(t *testing.T) {
	at, speed, remainingAtWrite := 70.0, 4.0, 0.0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		out := any(map[string]any{"processID": 42})
		switch q.URL.Path {
		case "/liveclientdata/allgamedata":
			time.Sleep(100 * time.Millisecond)
			at = 70.4
			out = map[string]any{"gameData": map[string]any{"gameTime": 70}, "allPlayers": []any{map[string]any{"riotId": "Player#KR1", "team": "ORDER", "isDead": true, "respawnTimer": 5.4}}}
		case "/replay/playback":
			if q.Method == "POST" {
				speed = 1
				remainingAtWrite = 75.4 - at
			}
			out = map[string]any{"time": at, "speed": speed}
		}
		json.NewEncoder(w).Encode(out)
	}))
	defer server.Close()
	r, err := New(Config{ReplayURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	x := speedTimeline{from: 60, to: 90, at: 60, speed: 4, policy: deathSpeedPolicy{tracking: true, since: 60}}
	if err := r.updateAutoSpeed(context.Background(), 42, player{Team: "ORDER"}, RiotID{GameName: "Player", TagLine: "KR1"}, &x); err != nil {
		t.Fatal(err)
	}
	if speed != 1 || remainingAtWrite < 3 {
		t.Fatalf("late respawn restoration: speed %v remaining %v", speed, remainingAtWrite)
	}
}

func TestSpeedTimelineCoverage(t *testing.T) {
	x := speedTimeline{from: 60, to: 90, at: 60, speed: 1}
	x.change(70, 4)
	x.change(80, 1)
	s := x.finish()
	got, err := ExpectedMediaDuration(s, 60, 90)
	if err != nil || got != 22.5 {
		t.Fatalf("%v %v", got, err)
	}
	for _, s := range [][]SpeedSegment{nil, {{60, 70, 1}, {71, 90, 4}}, {{60, 90, 0}}, {{60, 90, math.NaN()}}, {{60, 89, 1}}, {{60, 91, 1}}, {{60, 60, 1}}} {
		if _, err := ExpectedMediaDuration(s, 60, 90); err == nil {
			t.Fatal("accepted invalid timeline", s)
		}
	}
}

func TestSpeedControllerReadbackAndOwnership(t *testing.T) {
	for _, mode := range []string{"ok", "missing", "stale", "wrong-player", "ignored", "write-error", "owner-change"} {
		t.Run(mode, func(t *testing.T) {
			speed := 4.0
			writes, owners := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
				out := any(map[string]any{})
				switch q.URL.Path {
				case "/replay/game":
					owners++
					pid := 42
					if mode == "owner-change" && owners > 1 {
						pid = 43
					}
					out = map[string]any{"processID": pid}
				case "/replay/playback":
					if q.Method == "POST" {
						writes++
						if mode == "write-error" {
							w.WriteHeader(500)
							return
						}
						if mode != "ignored" {
							speed = 1
						}
					}
					out = map[string]any{"time": 70, "speed": speed}
				case "/liveclientdata/allgamedata":
					if mode == "missing" {
						w.WriteHeader(503)
						return
					}
					at := 70
					if mode == "stale" {
						at = 0
					}
					name := "Player#KR1"
					if mode == "wrong-player" {
						name = "Other#KR1"
					}
					out = map[string]any{"gameData": map[string]any{"gameTime": at}, "allPlayers": []any{map[string]any{"riotId": name, "team": "ORDER", "isDead": false}}}
				}
				json.NewEncoder(w).Encode(out)
			}))
			defer server.Close()
			r, err := New(Config{ReplayURL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			x := speedTimeline{from: 60, to: 90, at: 60, speed: 4}
			err = r.updateAutoSpeed(context.Background(), 42, player{Team: "ORDER"}, RiotID{GameName: "Player", TagLine: "KR1"}, &x)
			if mode == "ignored" || mode == "write-error" || mode == "owner-change" {
				if err == nil {
					t.Fatal("accepted unsafe transition")
				}
				if mode == "owner-change" && writes != 0 {
					t.Fatal("wrote to wrong owner")
				}
			} else if err != nil || speed != 1 || len(x.segments) != 1 {
				t.Fatalf("restore failed %v %v", err, x)
			}
		})
	}
}
