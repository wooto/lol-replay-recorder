package recorder

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRiotIDAndTargetSelection(t *testing.T) {
	id, err := ParseRiotID("플레이어#KR1")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"name", "#KR1", "name#", "name#tag#extra"} {
		if _, err := ParseRiotID(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	players := []player{{GameName: id.GameName, TagLine: "OTHER", Team: "ORDER"}, {GameName: id.GameName, TagLine: "KR1", Team: "CHAOS"}}
	slot, target, err := locateTarget(players, id)
	if err != nil || slot != 5 {
		t.Fatalf("incorrect identity/slot: %d %v", slot, err)
	}
	attached := true
	if locked(renderState{SelectionName: id.GameName, CameraAttached: &attached}, target, id) {
		t.Fatal("accepted ambiguous bare game name as camera proof")
	}
	if !locked(renderState{SelectionName: id.String(), CameraAttached: &attached}, target, id) {
		t.Fatal("rejected full camera identity")
	}
	players = append(players, players[1])
	if _, _, err := locateTarget(players, id); !errors.Is(err, ErrAmbiguousTarget) {
		t.Fatal(err)
	}
	if _, _, err := locateTarget([]player{{GameName: id.GameName, Team: "ORDER"}}, id); !errors.Is(err, ErrTargetNotFound) {
		t.Fatal("matched incomplete name")
	}
}
func TestLoopbackTransport(t *testing.T) {
	for _, base := range []string{"https://example.com:2999", "https://localhost:2999", "https://127.0.0.1@evil.example", "https://127.0.0.1:2999/path", "https://127.0.0.1:2999?token=secret", "https://127.0.0.1:0", "https://127.0.0.1:65536", "https://127.0.0.1:2999?"} {
		if _, err := newAPI(base, time.Second, false); err == nil {
			t.Fatalf("accepted origin %q", base)
		}
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"processID":1}`)) }))
	defer server.Close()
	api, err := newAPI(server.URL, time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	defer api.close()
	var game gameState
	if err = api.request(context.Background(), "GET", "/replay/game", nil, &game); err != nil || game.PID != 1 {
		t.Fatalf("isolated game certificate connection failed: %v", err)
	}
}
func TestRedirectIsNotFollowed(t *testing.T) {
	contacted := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { contacted = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	api, _ := newAPI(source.URL, time.Second, false)
	defer api.close()
	err := api.request(context.Background(), "GET", "/replay/game", nil, new(gameState))
	if err == nil || contacted {
		t.Fatal("redirect followed")
	}
}
func TestInvalidResponsesAreErrors(t *testing.T) {
	for _, body := range []string{`not json`, `{} {}`, `{"processID":1} trailing`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			api, _ := newAPI(server.URL, time.Second, false)
			defer api.close()
			if err := api.request(context.Background(), "GET", "/replay/game", nil, new(gameState)); err == nil {
				t.Fatal("invalid JSON accepted")
			}
		})
	}
}

func TestOversizedWhitespaceCannotHideTrailingResponseData(t *testing.T) {
	body := `{"processID":1}` + strings.Repeat(" ", 4<<20) + `{"processID":2}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	api, err := newAPI(server.URL, time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	defer api.close()
	if err := api.request(context.Background(), "GET", "/replay/game", nil, new(gameState)); err == nil {
		t.Fatal("oversized response accepted after reader truncation")
	}
}
func TestProbeRejectsPartialOrUndecodableVideo(t *testing.T) {
	for _, body := range []string{
		`{"format":{"duration":"30"},"streams":[{"codec_type":"video","nb_read_frames":"100"}]}`,
		`{"format":{"duration":"NaN"},"streams":[{"codec_type":"video","nb_read_frames":"100"}]}`,
		`{"format":{"duration":"90"},"streams":[{"codec_type":"audio","nb_read_frames":"100"}]}`,
		`{"format":{"duration":"90"},"streams":[{"codec_type":"video","nb_read_frames":"0"}]}`,
	} {
		if err := validateProbe([]byte(body), 90); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	if err := validateProbe([]byte(`{"format":{"duration":"90.1"},"streams":[{"codec_type":"video","nb_read_frames":"5400"}],"packets":[{"pts_time":"0","duration_time":"0.017"},{"pts_time":"89.983","duration_time":"0.017"}]}`), 90); err != nil {
		t.Fatal(err)
	}
}
