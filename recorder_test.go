package recorder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestConnectionRefusedFromClosedLocalListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if conn != nil {
		conn.Close()
		t.Fatal("unexpected connection")
	}
	if !connectionRefused(err) {
		t.Fatalf("closed local port should permit launch: %v", err)
	}
	if connectionRefused(context.DeadlineExceeded) {
		t.Fatal("timeout must not permit launch")
	}
}

type fixture struct {
	roster                     []map[string]any
	expectedSelectionKey       uint16
	cameraProfile              bool
	cameraLockX                bool
	cameraLockY                bool
	cameraLockZ                bool
	cameraMoveSpeed            float64
	cameraLookSpeed            float64
	cameraControlsSet          bool
	cameraTrack                bool
	cameraOffsetTrack          bool
	followOffsets              []cameraVector
	selectionOffset            cameraVector
	pendingOffset              *cameraVector
	pendingOffsetReads         int
	pendingSequenceOffset      *cameraVector
	pendingSequenceOffsetReads int
	deathAckPending            bool
	deathAckPostIgnored        bool
	deathAckRenderReads        int
	deathAckLockedRead         bool
	deathAckStaleOffset        bool
	deathAckInitial            cameraVector
	deathAckCommanded          cameraVector
	deathAckEmptySeen          bool
	deathAckDeathSeen          bool
	deathAckDeathClock         float64
	deathAckReacquired         bool
	enforced                   bool
	restored                   bool
	sequence                   []struct {
		Time  float64 `json:"time"`
		Value string  `json:"value"`
	}
	mu                                            sync.Mutex
	launched, selected, closed, stopped, verified bool
	mode                                          string
	ticks                                         int
	path                                          string
	start, end                                    float64
	playbackTime                                  float64
}
type fakeDesktop struct{ f *fixture }
type fakeProcess struct{ f *fixture }

func (p fakeProcess) pid() int     { return 4242 }
func (p fakeProcess) exited() bool { return false }
func (p fakeProcess) close() error {
	p.f.mu.Lock()
	defer p.f.mu.Unlock()
	p.f.closed = true
	return nil
}
func (d fakeDesktop) acquire() (func(), error) { return func() {}, nil }
func (d fakeDesktop) launch(context.Context, Config, string) (replayProcess, error) {
	d.f.mu.Lock()
	defer d.f.mu.Unlock()
	d.f.launched = true
	return fakeProcess{d.f}, nil
}
func (d fakeDesktop) selectPlayer(_ context.Context, _ int, key uint16) error {
	d.f.mu.Lock()
	defer d.f.mu.Unlock()
	expected := d.f.expectedSelectionKey
	if expected == 0 {
		expected = '2'
	}
	if key != expected {
		return fmt.Errorf("wrong player key %d", key)
	}
	d.f.selected = true
	return nil
}

type fakeVerifier struct{ f *fixture }

func (v fakeVerifier) ready() error { return nil }
func (v fakeVerifier) verify(_ context.Context, path string, duration float64) error {
	v.f.mu.Lock()
	defer v.f.mu.Unlock()
	v.f.verified = true
	if v.f.mode == "native-empty-programs" {
		return validateProbe([]byte(`{"programs":[],"format":{"duration":"90"},"streams":[{"codec_type":"video","nb_read_frames":"2700"}],"packets":[{"pts_time":"0","duration_time":"0.033"},{"pts_time":"89.967","duration_time":"0.033"}]}`), duration)
	}
	if v.f.mode == "native-preroll" {
		if v.f.start != -5 {
			return validateProbe([]byte(`{"format":{"duration":"90"},"streams":[{"codec_type":"video","nb_read_frames":"2500"}],"packets":[{"pts_time":"0","duration_time":"0.033"},{"pts_time":"84.967","duration_time":"0.033"}]}`), duration)
		}
		return validateProbe([]byte(`{"format":{"duration":"90"},"streams":[{"codec_type":"video","nb_read_frames":"2700"}],"packets":[{"pts_time":"0","duration_time":"0.033"},{"pts_time":"89.967","duration_time":"0.033"}]}`), duration)
	}
	if v.f.mode == "native-video-short" {
		return validateProbe([]byte(`{"format":{"duration":"90"},"streams":[{"codec_type":"video","nb_read_frames":"1350"}],"packets":[{"pts_time":"0","duration_time":"0.033"},{"pts_time":"44.967","duration_time":"0.033"}]}`), duration)
	}
	if v.f.mode == "camera-offset" && (!v.f.cameraProfile || !v.f.cameraTrack) {
		return ErrCameraLock
	}
	if v.f.mode == "corrupt" {
		return ErrRecordingIncomplete
	}
	if v.f.mode == "interval" || v.f.mode == "interval-camera-jump" {
		if path != v.f.path || duration != 30 || v.f.start != 55 || v.f.end != 90 {
			return errors.New("interval range or duration was not preserved")
		}
		return validateProbe([]byte(`{"format":{"duration":"30"},"streams":[{"codec_type":"video","nb_read_frames":"900"}],"packets":[{"pts_time":"0","duration_time":"0.033"},{"pts_time":"29.967","duration_time":"0.033"}]}`), duration)
	}
	if v.f.mode == "full-interval" && (path != v.f.path || duration != 90 || v.f.start != -5 || v.f.end != 90) {
		return errors.New("full-length interval range or duration was not preserved")
	}
	if path != v.f.path || duration != 90 {
		return errors.New("incorrect output passed to verifier")
	}
	if v.f.mode == "native-wall-clock" {
		// This native-client fixture models the observed accelerated encoder:
		// a whole game yields shortened media, despite successful API progress.
		if v.f.enforced {
			return validateProbe([]byte(`{"format":{"duration":"45"},"streams":[{"codec_type":"video","nb_read_frames":"1350"}]}`), duration)
		}
		return validateProbe([]byte(`{"format":{"duration":"90"},"streams":[{"codec_type":"video","nb_read_frames":"2700"}],"packets":[{"pts_time":"0","duration_time":"0.033"},{"pts_time":"89.967","duration_time":"0.033"}]}`), duration)
	}
	return nil
}
func (f *fixture) serve(w http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	encode := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	switch request.URL.Path {
	case "/replay/game":
		if f.mode == "preflight-error" && !f.launched {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		pid := 0
		if f.launched {
			pid = 4242
		}
		if f.mode == "busy" {
			pid = 99
		}
		if f.mode == "wrong-process" && f.launched {
			pid = 99
		}
		encode(map[string]any{"processID": pid})
	case "/replay/playback":
		if f.mode == "encoder-clock" && request.Method == "POST" && f.path != "" {
			http.Error(w, "encoder owns playback clock", http.StatusConflict)
			return
		}
		if request.Method == "POST" {
			var body struct {
				Time   *float64 `json:"time"`
				Paused *bool    `json:"paused"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				http.Error(w, "bad request", 400)
				return
			}
			if body.Time != nil {
				f.playbackTime = *body.Time
			}
			encode(map[string]any{})
			return
		}
		length := 90
		if f.mode == "load-timeout" {
			length = 0
		}
		current := f.playbackTime
		if (f.mode == "reset-on-complete" || f.mode == "reset-lost-lock") && f.ticks >= 4 {
			current = 90
		}
		encode(map[string]any{"length": length, "time": current, "paused": true, "seeking": false})
	case "/liveclientdata/allgamedata":
		if f.roster != nil {
			encode(map[string]any{"gameData": map[string]any{"gameTime": float64(f.ticks) * 30}, "allPlayers": f.roster})
			return
		}
		var dead any = (f.mode == "death-respawn" || f.mode == "death-unattached" || f.mode == "death-other-player" || f.mode == "death-stale-data" || f.mode == "camera-follow-death-ack") && f.ticks == 2
		if f.mode == "death-unknown" {
			dead = nil
		}
		gameTime := float64(f.ticks) * 30
		if f.mode == "camera-follow-death-ack" && f.ticks == 2 {
			f.deathAckDeathSeen = true
			f.deathAckDeathClock = gameTime
		}
		if f.mode == "death-stale-data" {
			gameTime = 0
		}
		encode(map[string]any{"gameData": map[string]any{"gameTime": gameTime}, "allPlayers": []map[string]any{
			{"riotIdGameName": "Player", "riotIdTagLine": "WRONG", "summonerName": "Player#WRONG", "team": "ORDER"},
			{"riotIdGameName": "Player", "riotIdTagLine": "KR1", "summonerName": "Player#KR1", "team": "ORDER", "isDead": dead},
		}})
	case "/replay/render":
		if request.Method == "POST" {
			var raw map[string]json.RawMessage
			if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
				http.Error(w, "invalid render", 400)
				return
			}
			if raw["selectionOffset"] != nil {
				var offset struct{ X, Y, Z float64 }
				var rotation struct{ X, Y, Z float64 }
				var mode string
				json.Unmarshal(raw["selectionOffset"], &offset)
				json.Unmarshal(raw["cameraRotation"], &rotation)
				json.Unmarshal(raw["cameraMode"], &mode)
				if raw["cameraMode"] != nil {
					if f.mode == "camera-profile-offset-ignored" {
						encode(map[string]any{})
						return
					}
					f.cameraProfile = mode == "fps" && offset.X == 0 && offset.Y > 1400 && offset.Y < 1600 && offset.Z < -900 && offset.Z > -1100 && rotation.Y == 56
					var controls struct {
						LockX     bool    `json:"cameraLockX"`
						LockY     bool    `json:"cameraLockY"`
						LockZ     bool    `json:"cameraLockZ"`
						MoveSpeed float64 `json:"cameraMoveSpeed"`
						LookSpeed float64 `json:"cameraLookSpeed"`
					}
					data, _ := json.Marshal(raw)
					json.Unmarshal(data, &controls)
					f.cameraControlsSet = raw["cameraLockX"] != nil && raw["cameraLockY"] != nil && raw["cameraLockZ"] != nil && raw["cameraMoveSpeed"] != nil && raw["cameraLookSpeed"] != nil &&
						!controls.LockX && !controls.LockY && !controls.LockZ && controls.MoveSpeed == 0 && controls.LookSpeed == 0
					if f.mode != "camera-input-controls-ignored" {
						f.cameraLockX, f.cameraLockY, f.cameraLockZ = controls.LockX, controls.LockY, controls.LockZ
						f.cameraMoveSpeed, f.cameraLookSpeed = controls.MoveSpeed, controls.LookSpeed
					}
				}
				if f.mode == "camera-follow-ignored-offset" && raw["cameraMode"] == nil {
					encode(map[string]any{})
					return
				}
				if f.mode == "camera-follow-delayed-offset" && raw["cameraMode"] == nil {
					pending := cameraVector{X: offset.X, Y: offset.Y, Z: offset.Z}
					f.pendingOffset = &pending
					f.pendingOffsetReads = 2
					encode(map[string]any{})
					return
				}
				if f.mode == "camera-follow-death-ack" && raw["cameraMode"] == nil && !f.deathAckEmptySeen {
					// The target dies between the last locked render read and this
					// accepted-but-ignored dynamic offset update.
					f.deathAckPending = true
					f.deathAckPostIgnored = true
					f.deathAckInitial = f.selectionOffset
					f.deathAckCommanded = cameraVector{X: offset.X, Y: offset.Y, Z: offset.Z}
					f.deathAckRenderReads = 0
					encode(map[string]any{})
					return
				}
				f.selectionOffset = cameraVector{X: offset.X, Y: offset.Y, Z: offset.Z}
				if raw["cameraMode"] == nil {
					f.followOffsets = append(f.followOffsets, f.selectionOffset)
				}
				encode(map[string]any{})
				return
			}
			if f.mode == "ignored-api" {
				encode(map[string]any{})
				return
			}
			if f.mode == "api-selection" || f.mode == "respawn-race" || f.mode == "camera-follow-death-ack" {
				var body struct {
					Name string `json:"selectionName"`
				}
				data, _ := json.Marshal(raw)
				if err := json.Unmarshal(data, &body); err != nil {
					t := "invalid selection"
					http.Error(w, t, 400)
					return
				}
				f.selected = body.Name == "Player#KR1"
				if f.mode == "respawn-race" && f.ticks == 2 {
					f.restored = true
				}
				encode(map[string]any{})
				return
			}
			http.Error(w, "fixture requires keyboard selection", http.StatusMethodNotAllowed)
			return
		}
		if f.pendingOffset != nil {
			if f.pendingOffsetReads > 0 {
				f.pendingOffsetReads--
			} else {
				f.selectionOffset = *f.pendingOffset
				f.followOffsets = append(f.followOffsets, f.selectionOffset)
				f.pendingOffset = nil
			}
		}
		if f.pendingSequenceOffset != nil {
			if f.pendingSequenceOffsetReads > 0 {
				f.pendingSequenceOffsetReads--
			} else {
				f.selectionOffset = *f.pendingSequenceOffset
				f.followOffsets = append(f.followOffsets, f.selectionOffset)
				f.pendingSequenceOffset = nil
			}
		}
		attached := f.selected && f.mode != "target-lock" && !(f.mode == "lost-lock" && f.ticks >= 2)
		if f.mode == "reset-lost-lock" && f.ticks >= 4 {
			attached = false
		}
		name := "Player#KR1"
		if f.mode == "respawn-race" && f.ticks == 2 && !f.restored {
			name = ""
		}
		if f.mode == "death-respawn" && f.ticks == 2 {
			name = ""
		}
		if f.mode == "camera-follow-death-ack" && f.ticks == 2 {
			name = ""
		}
		if f.ticks == 2 {
			switch f.mode {
			case "death-unattached":
				name = ""
				attached = false
			case "death-other-player":
				name = "Player#WRONG"
			case "death-unknown", "death-stale-data":
				name = ""
			}
		}
		camera := map[string]any{"selectionName": name, "cameraAttached": attached,
			"cameraMode": "fps", "selectionOffset": f.selectionOffset, "cameraRotation": map[string]any{"x": 0, "y": 56, "z": 0},
			"cameraLockX": f.cameraLockX, "cameraLockY": f.cameraLockY, "cameraLockZ": f.cameraLockZ,
			"cameraMoveSpeed": f.cameraMoveSpeed, "cameraLookSpeed": f.cameraLookSpeed}
		if f.mode == "camera-follow" || f.mode == "camera-follow-ignored-offset" || f.mode == "camera-follow-sequence-ignored" || f.mode == "camera-follow-delayed-offset" || f.mode == "camera-follow-death-ack" {
			targetX := 5000 + float64(f.ticks)*20
			cameraPosition := cameraVector{X: targetX + f.selectionOffset.X, Y: 100 + f.selectionOffset.Y, Z: 5000 + f.selectionOffset.Z}
			camera["cameraPosition"] = cameraPosition
		} else {
			targetX := 5000.0
			if f.mode == "interval-camera-jump" && f.playbackTime >= 60 {
				targetX += 200
			}
			camera["cameraPosition"] = cameraVector{X: targetX + f.selectionOffset.X, Y: 100 + f.selectionOffset.Y, Z: 5000 + f.selectionOffset.Z}
		}
		if f.mode == "camera-profile-ignored" {
			delete(camera, "selectionOffset")
		}
		if f.mode == "camera-position-missing" {
			delete(camera, "cameraPosition")
		}
		if f.mode == "camera-input-controls-drift" && f.ticks >= 2 {
			camera["cameraMoveSpeed"] = 1
		}
		if f.mode == "camera-profile-drift" && f.ticks >= 2 {
			camera["selectionOffset"] = map[string]any{"x": 0, "y": 0, "z": 0}
		}
		if (f.mode == "camera-offset-drift-bounded" || f.mode == "camera-offset-drift-large") && f.ticks == 2 {
			drift := 100.0
			if f.mode == "camera-offset-drift-large" {
				drift = 600
			}
			camera["selectionOffset"] = cameraVector{X: f.selectionOffset.X + drift, Y: f.selectionOffset.Y, Z: f.selectionOffset.Z}
		}
		if f.mode == "camera-follow-death-ack" && f.deathAckPending {
			f.deathAckRenderReads++
			if f.deathAckRenderReads == 1 {
				f.deathAckLockedRead = name == "Player#KR1" && attached
				f.deathAckStaleOffset = f.selectionOffset == f.deathAckInitial && f.selectionOffset != f.deathAckCommanded
			} else if f.deathAckRenderReads == 2 {
				camera["selectionName"] = ""
				camera["cameraAttached"] = true
				f.deathAckEmptySeen = true
				f.deathAckPending = false
			}
		}
		if f.mode == "camera-follow-death-ack" && f.ticks >= 3 && name == "Player#KR1" && attached {
			f.deathAckReacquired = true
		}
		encode(camera)
	case "/replay/sequence":
		if f.mode == "sequence-error" {
			http.Error(w, "sequence unavailable", 500)
			return
		}
		var body struct {
			Offset []struct {
				Time  float64      `json:"time"`
				Value cameraVector `json:"value"`
				Blend string       `json:"blend"`
			} `json:"selectionOffset"`
			Rotation []struct {
				Time  float64      `json:"time"`
				Value cameraVector `json:"value"`
				Blend string       `json:"blend"`
			} `json:"cameraRotation"`
			Selection []struct {
				Time  float64 `json:"time"`
				Value string  `json:"value"`
			} `json:"selectionName"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(w, "invalid sequence", 400)
			return
		}
		f.sequence = body.Selection
		f.cameraOffsetTrack = len(body.Offset) == 2
		f.cameraTrack = len(body.Selection) == 2 && len(body.Rotation) == 2 && body.Rotation[0].Value.Y == 56 && body.Rotation[1].Value.Y == 56
		if len(body.Offset) == 2 {
			endpoint := body.Offset[1].Value
			switch {
			case f.mode == "camera-follow-death-ack" && !f.deathAckEmptySeen:
				// The target dies after the last locked read; the API accepts the
				// sequence update but cannot apply it to the vanished selection.
				f.deathAckPending = true
				f.deathAckPostIgnored = true
				f.deathAckInitial = f.selectionOffset
				f.deathAckCommanded = endpoint
				f.deathAckRenderReads = 0
			case f.mode != "camera-follow-sequence-ignored":
				pending := endpoint
				f.pendingSequenceOffset = &pending
				f.pendingSequenceOffsetReads = 1
				if f.mode == "camera-follow-delayed-offset" {
					f.pendingSequenceOffsetReads = 2
				}
			}
		}
		encode(map[string]any{})
	case "/replay/recording":
		if request.Method == "POST" {
			var body struct {
				Enforced  bool    `json:"enforceFrameRate"`
				Recording bool    `json:"recording"`
				Path      string  `json:"path"`
				Start     float64 `json:"startTime"`
				End       float64 `json:"endTime"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				http.Error(w, "bad request", 400)
				return
			}
			if body.Recording {
				f.enforced = body.Enforced
				f.path = body.Path
				f.start = body.Start
				f.end = body.End
			} else {
				f.stopped = true
			}
			encode(map[string]any{})
			return
		}
		if f.path == "" {
			encode(map[string]any{"recording": false})
			return
		}
		f.ticks++
		if f.mode == "delayed-start" && f.ticks == 1 {
			encode(map[string]any{"recording": false})
			return
		}
		active := f.ticks <= 3
		current := float64(f.ticks) * 30
		if f.mode == "interval" || f.mode == "interval-camera-jump" {
			current = f.playbackTime + float64(f.ticks)*10
		}
		if !active {
			current = f.end
			if f.mode != "interval" && f.mode != "interval-camera-jump" {
				current = 90
			}
			if f.mode == "interval" || f.mode == "interval-camera-jump" {
				f.playbackTime = f.end
			}
		}
		if f.mode == "never-started" {
			active = false
			current = 90
		}
		if f.mode == "early-stop" && f.ticks >= 2 {
			active = false
			current = 30
		}
		if f.mode == "missing-state" {
			encode(map[string]any{"path": f.path})
			return
		}
		path := f.path
		if f.mode == "wrong-output" {
			path = f.path + ".other.webm"
		}
		if !active {
			if err := os.WriteFile(f.path, []byte("fixture video"), 0600); err != nil {
				http.Error(w, "fixture output failed", 500)
				return
			}
		}
		if !active && (f.mode == "reset-on-complete" || f.mode == "reset-lost-lock") {
			encode(map[string]any{"recording": false, "path": "", "currentTime": 0, "startTime": -1, "endTime": -1})
			return
		}
		if !active && f.mode == "native-complete" {
			encode(map[string]any{"recording": false, "path": path, "currentTime": f.end, "startTime": f.start, "endTime": -1})
			return
		}
		encode(map[string]any{"recording": active, "path": path, "startTime": f.start, "endTime": f.end, "currentTime": current})
	default:
		http.NotFound(w, request)
	}
}
func testRecorder(t *testing.T, mode string) (*Recorder, Request, *fixture) {
	t.Helper()
	f := &fixture{mode: mode, selectionOffset: cameraVector{X: 0, Y: 1492.267578125, Z: -1006.5472412109375}}
	if mode == "camera-input-controls-ignored" {
		f.cameraLockX, f.cameraLockY, f.cameraLockZ = true, true, true
		f.cameraMoveSpeed, f.cameraLookSpeed = 100, 1
	}
	if mode == "camera-profile-offset-ignored" {
		f.selectionOffset.X = 100
	}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	r, err := New(Config{ReplayURL: server.URL, PollInterval: time.Millisecond, LaunchTimeout: 300 * time.Millisecond, FinalizeTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	r.desktop = fakeDesktop{f}
	r.verifier = fakeVerifier{f}
	folder := t.TempDir()
	replay := filepath.Join(folder, "리플레이 with spaces.rofl")
	if err := os.WriteFile(replay, []byte("local fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	return r, Request{ReplayPath: replay, Target: RiotID{"Player", "KR1"}, OutputPath: filepath.Join(folder, "full 경기.webm")}, f
}
func TestRecordFull(t *testing.T) {
	r, request, f := testRecorder(t, "")
	result, err := r.RecordFull(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != request.OutputPath || result.DurationSeconds != 90 || result.Target != request.Target {
		t.Fatalf("incorrect result: %+v", result)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.selected || !f.verified || !f.closed || f.start != -5 || f.end != 90 || !f.cameraControlsSet ||
		f.cameraLockX || f.cameraLockY || f.cameraLockZ || f.cameraMoveSpeed != 0 || f.cameraLookSpeed != 0 {
		t.Fatalf("incomplete lifecycle: %+v", f)
	}
	if _, err := os.Stat(request.OutputPath + ".recorder-lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("output reservation leaked")
	}
}

func TestRecordFullSelectsPlayerByOrderWithinTeam(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		teams  []string
		target int
		key    uint16
	}{
		{"blue-first-after-red", []string{"CHAOS", "ORDER", "CHAOS", "ORDER"}, 1, '1'},
		{"red-first-after-blue", []string{"ORDER", "CHAOS", "ORDER"}, 1, 'Q'},
		{"red-second-interleaved", []string{"CHAOS", "ORDER", "CHAOS", "ORDER"}, 2, 'W'},
		{"blue-third-interleaved", []string{"ORDER", "CHAOS", "ORDER", "CHAOS", "ORDER"}, 4, '3'},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			r, request, f := testRecorder(t, "ignored-api")
			f.expectedSelectionKey = scenario.key
			for i, team := range scenario.teams {
				id := fmt.Sprintf("Other%d#KR1", i)
				if i == scenario.target {
					id = "Player#KR1"
				}
				// Duplicate champion names must not replace Riot-ID identification.
				f.roster = append(f.roster, map[string]any{"riotId": id, "summonerName": id, "team": team, "championName": "Kayle", "isDead": false})
			}
			result, err := r.RecordFull(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if result.Target != request.Target || !f.selected || !f.verified {
				t.Fatal("incorrect target or incomplete recording")
			}
		})
	}
}

type fixtureHotkeys struct {
	keys        [10]uint16
	events      []string
	ignoreApply bool
	backupErr   error
}

func (h *fixtureHotkeys) Read(context.Context) ([10]uint16, error) {
	h.events = append(h.events, "read")
	return h.keys, nil
}
func (h *fixtureHotkeys) Backup(context.Context) error {
	h.events = append(h.events, "backup")
	return h.backupErr
}
func (h *fixtureHotkeys) Apply(_ context.Context, keys [10]uint16) error {
	h.events = append(h.events, "apply")
	if !h.ignoreApply {
		h.keys = keys
	}
	return nil
}
func TestRecordFullRejectsMismatchedHotkeysBeforeLaunch(t *testing.T) {
	r, request, f := testRecorder(t, "")
	h := &fixtureHotkeys{keys: [10]uint16{'9', '2', '3', '4', '5', 'Q', 'W', 'E', 'R', 'T'}}
	r.config.Hotkeys = h
	_, err := r.RecordFull(context.Background(), request)
	if !errors.Is(err, ErrHotkeySettings) || f.launched {
		t.Fatalf("mismatched keys must stop before launch: err=%v launched=%v", err, f.launched)
	}
	if fmt.Sprint(h.events) != "[read]" {
		t.Fatalf("check-only changed settings: %v", h.events)
	}
}

func TestRecordFullConfiguresHotkeysWithBackupAndReadback(t *testing.T) {
	r, request, f := testRecorder(t, "")
	h := &fixtureHotkeys{keys: [10]uint16{'9', '2', '3', '4', '5', 'Q', 'W', 'E', 'R', 'T'}}
	r.config.Hotkeys, r.config.ConfigureHotkeys = h, true
	_, err := r.RecordFull(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(h.events) != "[read backup apply read]" || !f.launched {
		t.Fatalf("must back up, apply and verify before launch: events=%v launched=%v", h.events, f.launched)
	}
}

func TestRecordFullDoesNotConfigureUnknownHotkeys(t *testing.T) {
	r, request, f := testRecorder(t, "")
	h := &fixtureHotkeys{}
	r.config.Hotkeys, r.config.ConfigureHotkeys = h, true
	_, err := r.RecordFull(context.Background(), request)
	if !errors.Is(err, ErrHotkeySettings) || f.launched || fmt.Sprint(h.events) != "[read]" {
		t.Fatalf("unknown settings must not be rewritten: err=%v events=%v launched=%v", err, h.events, f.launched)
	}
}

func TestRecordFullDoesNotConfigureConflictingSlotKeys(t *testing.T) {
	r, request, f := testRecorder(t, "")
	h := &fixtureHotkeys{keys: [10]uint16{'2', '2', '3', '4', '5', 'Q', 'W', 'E', 'R', 'T'}}
	r.config.Hotkeys, r.config.ConfigureHotkeys = h, true
	_, err := r.RecordFull(context.Background(), request)
	if !errors.Is(err, ErrHotkeySettings) || f.launched || fmt.Sprint(h.events) != "[read]" {
		t.Fatalf("conflicting slots must stop: err=%v events=%v launched=%v", err, h.events, f.launched)
	}
}

func TestRecordFullHotkeyApplyFailuresStopBeforeLaunch(t *testing.T) {
	for _, scenario := range []string{"backup-failed", "apply-ignored", "backup-cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			r, request, f := testRecorder(t, "")
			h := &fixtureHotkeys{keys: [10]uint16{'9', '2', '3', '4', '5', 'Q', 'W', 'E', 'R', 'T'}}
			if scenario == "backup-failed" {
				h.backupErr = errors.New("backup unavailable")
			}
			if scenario == "backup-cancelled" {
				h.backupErr = context.Canceled
			}
			if scenario == "apply-ignored" {
				h.ignoreApply = true
			}
			r.config.Hotkeys, r.config.ConfigureHotkeys = h, true
			result, err := r.RecordFull(context.Background(), request)
			if !errors.Is(err, ErrHotkeySettings) || f.launched || result.Path != "" {
				t.Fatalf("failed settings must prevent launch and result: err=%v launched=%v result=%+v", err, f.launched, result)
			}
			want := "[read backup]"
			if scenario == "apply-ignored" {
				want = "[read backup apply read]"
			}
			if fmt.Sprint(h.events) != want {
				t.Fatalf("unexpected settings operations: %v", h.events)
			}
			if scenario == "backup-cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation cause")
			}
		})
	}
}
func TestFailedRecordingsDoNotReturnFullResult(t *testing.T) {
	tests := []struct {
		mode  string
		cause error
		stage Stage
	}{
		{"preflight-error", nil, StageValidate},
		{"busy", ErrClientBusy, StageValidate},
		{"wrong-process", nil, StageLoad},
		{"load-timeout", context.DeadlineExceeded, StageLoad},
		{"target-lock", ErrCameraLock, StageTarget},
		{"lost-lock", ErrCameraLock, StageRecord},
		{"never-started", ErrRecordingIncomplete, StageRecord},
		{"early-stop", ErrRecordingIncomplete, StageRecord},
		{"missing-state", nil, StageRecord},
		{"wrong-output", nil, StageRecord},
		{"corrupt", ErrRecordingIncomplete, StageFinalize},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			r, request, f := testRecorder(t, test.mode)
			result, err := r.RecordFull(context.Background(), request)
			if err == nil || result.Path != "" {
				t.Fatalf("unexpected successful result: %+v %v", result, err)
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("want %v, got %v", test.cause, err)
			}
			var failure *Error
			if !errors.As(err, &failure) || failure.Stage != test.stage {
				t.Fatalf("wrong stage: %v", err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.launched && !f.closed {
				t.Fatal("owned process leaked")
			}
			if test.stage == StageRecord && !f.stopped {
				t.Fatal("failed recording was not stopped")
			}
		})
	}
}
func TestDelayedRecordingStart(t *testing.T) {
	r, request, _ := testRecorder(t, "delayed-start")
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatalf("delayed start should still record the full replay: %v", err)
	}
}
func TestCancellationStopsOnlyOwnedRecording(t *testing.T) {
	r, request, f := testRecorder(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.config.OnProgress = func(progress Progress) {
		if progress.Stage == StageRecord && progress.CurrentSeconds > 0 {
			cancel()
		}
	}
	_, err := r.RecordFull(ctx, request)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context cancellation: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.stopped || !f.closed {
		t.Fatal("cancellation leaked recording or process")
	}
}
func TestOutputIsNeverOverwritten(t *testing.T) {
	r, request, f := testRecorder(t, "")
	original := []byte("keep this")
	if err := os.WriteFile(request.OutputPath, original, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := r.RecordFull(context.Background(), request)
	if !errors.Is(err, ErrOutputExists) {
		t.Fatal(err)
	}
	var failure *Error
	if !errors.As(err, &failure) || failure.PartialPath != "" {
		t.Fatal("preexisting output must not be reported as this recording's partial output")
	}
	data, _ := os.ReadFile(request.OutputPath)
	if string(data) != string(original) || f.launched {
		t.Fatal("existing output modified or game launched")
	}
}
func TestReentrantRecordingIsBusy(t *testing.T) {
	r, request, _ := testRecorder(t, "")
	var nestedErr error
	r.config.OnProgress = func(progress Progress) {
		if progress.Stage == StageValidate {
			_, nestedErr = r.RecordFull(context.Background(), request)
		}
	}
	if _, err := r.RecordFull(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(nestedErr, ErrBusy) {
		t.Fatalf("want busy, got %v", nestedErr)
	}
}
