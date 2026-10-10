// Package spectator controls Riot's local Replay API in a spectator/replay game.
// It is distinct from observer's raw stream capture and LCU's launch handoff.
package spectator

import (
	"context"
	"errors"
	"math"

	"github.com/wooto/lol-replay-recorder/internal/localhttp"
)

type Config = localhttp.Config
type StatusError = localhttp.StatusError
type Client struct{ http *localhttp.Client }
type Game struct {
	ProcessID int `json:"processID"`
}
type Playback struct {
	Time    float64 `json:"time"`
	Length  float64 `json:"length"`
	Speed   float64 `json:"speed"`
	Paused  bool    `json:"paused"`
	Seeking bool    `json:"seeking"`
}

// PlaybackUpdate uses pointers so omitted fields retain their current values.
type PlaybackUpdate struct {
	Time   *float64 `json:"time,omitempty"`
	Speed  *float64 `json:"speed,omitempty"`
	Paused *bool    `json:"paused,omitempty"`
}

func New(config Config) (*Client, error) {
	h, err := localhttp.New(config, "https://127.0.0.1:2999", false)
	if err != nil {
		return nil, err
	}
	return &Client{http: h}, nil
}
func (c *Client) Close() { c.http.Close() }
func (c *Client) Game(ctx context.Context) (Game, error) {
	var game Game
	err := c.http.Request(ctx, "GET", "/replay/game", nil, &game)
	if err == nil && game.ProcessID <= 0 {
		err = errors.New("Replay API did not identify a game process")
	}
	return game, err
}
func (c *Client) Playback(ctx context.Context) (Playback, error) {
	var state Playback
	err := c.http.Request(ctx, "GET", "/replay/playback", nil, &state)
	return state, err
}
func (c *Client) UpdatePlayback(ctx context.Context, update PlaybackUpdate) error {
	for _, n := range []*float64{update.Time, update.Speed} {
		if n != nil && (math.IsNaN(*n) || math.IsInf(*n, 0) || *n < 0) {
			return errors.New("playback values must be finite and nonnegative")
		}
	}
	if update.Speed != nil && *update.Speed == 0 {
		return errors.New("speed must be positive; use Paused to pause")
	}
	if update.Time == nil && update.Speed == nil && update.Paused == nil {
		return errors.New("playback update is empty")
	}
	return c.http.Request(ctx, "POST", "/replay/playback", update, nil)
}
