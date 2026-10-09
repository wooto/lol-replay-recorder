package recorder

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type replayAPI struct {
	base   string
	client *http.Client
}

func newAPI(base string, timeout time.Duration, strict bool) (*replayAPI, error) {
	if base == "" {
		base = "https://127.0.0.1:2999"
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, errors.New("invalid ReplayURL")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("ReplayURL must be a numeric loopback HTTP(S) origin without credentials")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: !strict} // #nosec G402 -- validated numeric loopback, isolated transport, redirects disabled.
	client := &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("replay redirects are not allowed") }}
	return &replayAPI{base: strings.TrimRight(base, "/"), client: client}, nil
}
func (a *replayAPI) close() { a.client.CloseIdleConnections() }
func (a *replayAPI) request(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Replay API %s %s: HTTP %d", method, path, response.StatusCode)
	}
	if out == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return err
	}
	const maxJSONBytes = 4 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxJSONBytes+1))
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) > maxJSONBytes {
		return errors.New("Replay API response exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("Replay API response contains trailing or oversized data")
	}
	return nil
}

type gameState struct {
	PID int `json:"processID"`
}
type playbackState struct {
	Time    float64 `json:"time"`
	Length  float64 `json:"length"`
	Paused  bool    `json:"paused"`
	Seeking bool    `json:"seeking"`
}
type renderState struct {
	CameraMode      string        `json:"cameraMode"`
	SelectionOffset *cameraVector `json:"selectionOffset"`
	CameraRotation  *cameraVector `json:"cameraRotation"`
	CameraPosition  *cameraVector `json:"cameraPosition"`
	SelectionName   string        `json:"selectionName"`
	CameraAttached  *bool         `json:"cameraAttached"`
}

type cameraVector struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

func elevatedCamera(state renderState) bool {
	return cameraPoseValid(state) && cameraOffsetWithinRange(state)
}

func cameraPoseValid(state renderState) bool {
	if state.CameraMode != "fps" || state.SelectionOffset == nil || state.CameraRotation == nil || state.CameraPosition == nil {
		return false
	}
	near := func(a, b float64) bool { return math.Abs(a-b) <= 0.5 }
	o, r := state.SelectionOffset, state.CameraRotation
	p := state.CameraPosition
	return finiteCameraVector(*o) && finiteCameraVector(*r) && finiteCameraVector(*p) &&
		near(o.Y, baseCameraOffset.Y) &&
		near(r.X, 0) && near(r.Y, 56) && near(r.Z, 0)
}

func cameraOffsetWithinRange(state renderState) bool {
	return state.SelectionOffset != nil && horizontalDistance(*state.SelectionOffset, baseCameraOffset) <= maxCameraOffsetDrift
}

type recordingState struct {
	Recording *bool   `json:"recording"`
	Path      string  `json:"path"`
	Start     float64 `json:"startTime"`
	End       float64 `json:"endTime"`
	Current   float64 `json:"currentTime"`
}
type player struct {
	IsDead       *bool  `json:"isDead"`
	NameUnique   bool   `json:"-"`
	RiotID       string `json:"riotId"`
	GameName     string `json:"riotIdGameName"`
	TagLine      string `json:"riotIdTagLine"`
	SummonerName string `json:"summonerName"`
	Team         string `json:"team"`
}

func (p player) identity() string {
	if p.GameName != "" && p.TagLine != "" {
		return p.GameName + "#" + p.TagLine
	}
	if strings.Contains(p.RiotID, "#") {
		return p.RiotID
	}
	if strings.Contains(p.SummonerName, "#") {
		return p.SummonerName
	}
	return ""
}

type gameData struct {
	Players []player `json:"allPlayers"`
	Clock   struct {
		Time *float64 `json:"gameTime"`
	} `json:"gameData"`
}

func locateTarget(players []player, id RiotID) (int, player, error) {
	counts := map[string]int{"ORDER": 0, "CHAOS": 0}
	found := -1
	var target player
	for _, p := range players {
		base := 0
		switch p.Team {
		case "ORDER":
		case "CHAOS":
			base = 5
		default:
			return -1, player{}, errors.New("unexpected player team")
		}
		slot := counts[p.Team]
		counts[p.Team]++
		if slot >= 5 {
			return -1, player{}, errors.New("replay has more than five players on a team")
		}
		if strings.EqualFold(p.identity(), id.String()) {
			if found >= 0 {
				return -1, player{}, ErrAmbiguousTarget
			}
			found = base + slot
			target = p
		}
	}
	if found < 0 {
		return -1, player{}, ErrTargetNotFound
	}
	matches := 0
	for _, p := range players {
		name := p.GameName
		if name == "" {
			name, _, _ = strings.Cut(p.identity(), "#")
		}
		if strings.EqualFold(name, id.GameName) {
			matches++
		}
	}
	target.NameUnique = matches == 1
	return found, target, nil
}
func locked(state renderState, p player, id RiotID) bool {
	return state.CameraAttached != nil && *state.CameraAttached && state.SelectionName != "" &&
		(strings.EqualFold(state.SelectionName, id.String()) ||
			(strings.Contains(p.SummonerName, "#") && strings.EqualFold(state.SelectionName, p.SummonerName)) ||
			(p.NameUnique && (strings.EqualFold(state.SelectionName, id.GameName) || state.SelectionName == p.SummonerName)))
}
func finitePositive(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }
