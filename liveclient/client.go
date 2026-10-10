// Package liveclient reads Riot's documented Live Client Data API on loopback.
package liveclient

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/wooto/lol-replay-recorder/internal/localhttp"
)

type Config = localhttp.Config
type StatusError = localhttp.StatusError
type Client struct{ http *localhttp.Client }
type Player struct {
	RiotID       string            `json:"riotId"`
	SummonerName string            `json:"summonerName"`
	ChampionName string            `json:"championName"`
	Team         string            `json:"team"`
	IsDead       bool              `json:"isDead"`
	RespawnTimer float64           `json:"respawnTimer"`
	Level        int               `json:"level"`
	Scores       json.RawMessage   `json:"scores"`
	Items        []json.RawMessage `json:"items"`
}
type ActivePlayer struct {
	RiotID        string          `json:"riotId"`
	SummonerName  string          `json:"summonerName"`
	Level         int             `json:"level"`
	CurrentGold   float64         `json:"currentGold"`
	ChampionStats json.RawMessage `json:"championStats"`
	Abilities     json.RawMessage `json:"abilities"`
}
type Event struct {
	EventID    uint64   `json:"EventID"`
	EventName  string   `json:"EventName"`
	EventTime  float64  `json:"EventTime"`
	KillerName string   `json:"KillerName"`
	VictimName string   `json:"VictimName"`
	Assisters  []string `json:"Assisters"`
}
type Events struct {
	Events []Event `json:"Events"`
}
type GameData struct {
	GameTime  float64 `json:"gameTime"`
	GameMode  string  `json:"gameMode"`
	MapNumber int     `json:"mapNumber"`
}
type Snapshot struct {
	ActivePlayer ActivePlayer `json:"activePlayer"`
	AllPlayers   []Player     `json:"allPlayers"`
	Events       Events       `json:"events"`
	GameData     GameData     `json:"gameData"`
}

func New(config Config) (*Client, error) {
	h, err := localhttp.New(config, "https://127.0.0.1:2999", false)
	if err != nil {
		return nil, err
	}
	return &Client{http: h}, nil
}
func (c *Client) Close() { c.http.Close() }
func (c *Client) AllGameData(ctx context.Context) (Snapshot, error) {
	var data Snapshot
	err := c.http.Request(ctx, "GET", "/liveclientdata/allgamedata", nil, &data)
	if err == nil && (data.GameData.MapNumber <= 0 || data.GameData.GameMode == "") {
		err = errors.New("live snapshot has no game data")
	}
	return data, err
}
func (c *Client) Players(ctx context.Context) ([]Player, error) {
	var players []Player
	err := c.http.Request(ctx, "GET", "/liveclientdata/playerlist", nil, &players)
	return players, err
}
func (c *Client) ActivePlayer(ctx context.Context) (ActivePlayer, error) {
	var player ActivePlayer
	err := c.http.Request(ctx, "GET", "/liveclientdata/activeplayer", nil, &player)
	return player, err
}
func (c *Client) GameStats(ctx context.Context) (GameData, error) {
	var game GameData
	err := c.http.Request(ctx, "GET", "/liveclientdata/gamestats", nil, &game)
	return game, err
}
func (c *Client) Events(ctx context.Context, eventID uint64) (Events, error) {
	var events Events
	err := c.http.Request(ctx, "GET", "/liveclientdata/eventdata?eventID="+strconv.FormatUint(eventID, 10), nil, &events)
	return events, err
}
