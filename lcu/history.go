package lcu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type Summoner struct {
	PUUID      string `json:"puuid"`
	SummonerID uint64 `json:"summonerId"`
	GameName   string `json:"gameName"`
	TagLine    string `json:"tagLine"`
}
type Match struct {
	GameID       uint64            `json:"gameId"`
	GameVersion  string            `json:"gameVersion"`
	QueueID      int               `json:"queueId"`
	GameDuration uint64            `json:"gameDuration"`
	Participants []json.RawMessage `json:"participants"`
}
type MatchHistory struct {
	Games struct {
		GameCount int     `json:"gameCount"`
		Games     []Match `json:"games"`
	} `json:"games"`
}

func (c *Client) CurrentSummoner(ctx context.Context) (Summoner, error) {
	var result Summoner
	err := c.http.Request(ctx, "GET", "/lol-summoner/v1/current-summoner", nil, &result)
	if err == nil && strings.TrimSpace(result.PUUID) == "" {
		err = errors.New("current summoner has no PUUID")
	}
	return result, err
}

// MatchHistory requests the explicit inclusive index range used by LCU.
func (c *Client) MatchHistory(ctx context.Context, puuid string, begin, end int) (MatchHistory, error) {
	var result MatchHistory
	if strings.TrimSpace(puuid) == "" || begin < 0 || end < begin || end-begin > 100 {
		return result, errors.New("PUUID and a valid range of at most 100 indices required")
	}
	path := fmt.Sprintf("/lol-match-history/v1/products/lol/%s/matches?begIndex=%d&endIndex=%d", url.PathEscape(puuid), begin, end)
	err := c.http.Request(ctx, "GET", path, nil, &result)
	return result, err
}
