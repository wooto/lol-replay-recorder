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
	cameraProfile     bool
	cameraTrack       bool
	cameraOffsetTrack bool
	followOffsets     []cameraVector
	selectionOffset   cameraVector
	enforced          bool
	restored          bool
	sequence          []struct {
		Time  float64 `json:"time"`
		Value string  `json:"value"`
	}
	mu                                            sync.Mutex
	launched, selected, closed, stopped, verified bool
	mode                                          string
	ticks                                         int
	path                                          string
	start, end                                    float64
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
	if key != '2' {
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
		length := 90
		if f.mode == "load-timeout" {
			length = 0
		}
		current := 0
		if (f.mode == "reset-on-complete" || f.mode == "reset-lost-lock") && f.ticks >= 4 {
			current = 90
		}
		encode(map[string]any{"length": length, "time": current, "paused": true, "seeking": false})
	case "/liveclientdata/allgamedata":
		var dead any = (f.mode == "death-respawn" || f.mode == "death-unattached" || f.mode == "death-other-player" || f.mode == "death-stale-data") && f.ticks == 2
		if f.mode == "death-unknown" {
			dead = nil
		}
		gameTime := float64(f.ticks) * 30
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
				}
				if f.mode == "camera-follow-ignored-offset" && raw["cameraMode"] == nil {
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
			if f.mode == "api-selection" || f.mode == "respawn-race" {
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
			"cameraMode": "fps", "selectionOffset": f.selectionOffset, "cameraRotation": map[string]any{"x": 0, "y": 56, "z": 0}}
		if f.mode == "camera-follow" || f.mode == "camera-follow-ignored-offset" {
			targetX := 5000 + float64(f.ticks)*20
			cameraPosition := cameraVector{X: targetX + f.selectionOffset.X, Y: 100 + f.selectionOffset.Y, Z: 5000 + f.selectionOffset.Z}
			camera["cameraPosition"] = cameraPosition
		} else {
			camera["cameraPosition"] = cameraVector{X: 5000 + f.selectionOffset.X, Y: 100 + f.selectionOffset.Y, Z: 5000 + f.selectionOffset.Z}
		}
		if f.mode == "camera-profile-ignored" {
			delete(camera, "selectionOffset")
		}
		if f.mode == "camera-position-missing" {
			delete(camera, "cameraPosition")
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
		encode(camera)
	case "/replay/sequence":
		if f.mode == "sequence-error" {
			http.Error(w, "sequence unavailable", 500)
			return
		}
		var body struct {
			Offset []struct {
				Time  float64
				Value struct{ X, Y, Z float64 }
			} `json:"selectionOffset"`
			Rotation []struct {
				Time  float64
				Value struct{ X, Y, Z float64 }
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
		f.cameraOffsetTrack = len(body.Offset) > 0
		f.cameraTrack = len(body.Offset) == 0 && len(body.Rotation) == 2 && body.Rotation[0].Value.Y == 56 && body.Rotation[1].Value.Y == 56
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
		if !active {
			current = 90
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
			encode(map[string]any{"recording": false, "path": path, "currentTime": 90, "startTime": f.start, "endTime": -1})
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
	if !f.selected || !f.verified || !f.closed || f.start != -5 || f.end != 90 {
		t.Fatalf("incomplete lifecycle: %+v", f)
	}
	if _, err := os.Stat(request.OutputPath + ".recorder-lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("output reservation leaked")
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
