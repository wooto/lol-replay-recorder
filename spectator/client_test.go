package spectator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wooto/lol-replay-recorder/spectator"
)

func TestSpectatorPlaybackPauseAndSeek(t *testing.T) {
	state := struct {
		Time   float64 `json:"time"`
		Length float64 `json:"length"`
		Paused bool    `json:"paused"`
		Speed  float64 `json:"speed"`
	}{Time: 12, Length: 300, Speed: 1}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/replay/game":
			fmt.Fprint(w, `{"processID":4321}`)
		case "/replay/playback":
			if r.Method == "POST" {
				if json.NewDecoder(r.Body).Decode(&state) != nil {
					w.WriteHeader(400)
					return
				}
			}
			_ = json.NewEncoder(w).Encode(state)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, err := spectator.New(spectator.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	game, err := client.Game(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	playback, err := client.Playback(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	paused := true
	seek := 42.0
	err = client.UpdatePlayback(context.Background(), spectator.PlaybackUpdate{Paused: &paused, Time: &seek})
	if err != nil {
		t.Fatal(err)
	}
	after, err := client.Playback(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if game.ProcessID != 4321 || playback.Length != 300 || !after.Paused || after.Time != 42 {
		t.Fatal("pause/seek did not change playback")
	}
}

func TestSpectatorCameraUpdatePreservesUnspecifiedFields(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/replay/render" {
			w.WriteHeader(404)
			return
		}
		if r.Method == "POST" {
			var body map[string]json.RawMessage
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				w.WriteHeader(400)
				return
			}
			if len(body) != 2 || string(body["cameraMode"]) != `"fps"` || string(body["selectionName"]) != `"선수#KR1"` {
				w.WriteHeader(400)
				return
			}
		}
		fmt.Fprint(w, `{"cameraMode":"fps","selectionName":"선수#KR1","cameraAttached":true,"cameraPosition":{"x":100,"y":200,"z":300}}`)
	}))
	defer server.Close()
	client, err := spectator.New(spectator.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	mode, name := "fps", "선수#KR1"
	if err = client.UpdateRender(context.Background(), spectator.RenderUpdate{CameraMode: &mode, SelectionName: &name}); err != nil {
		t.Fatal(err)
	}
	state, err := client.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.CameraAttached || state.SelectionName != name || state.CameraPosition.Y != 200 {
		t.Fatal("camera state not decoded")
	}
}
