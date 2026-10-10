package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type Participant struct {
	PUUID          string `json:"puuid"`
	RiotIDGameName string `json:"riotIdGameName"`
	RiotIDTagline  string `json:"riotIdTagline"`
	ChampionName   string `json:"championName"`
	TeamID         int    `json:"teamId"`
	Kills          int    `json:"kills"`
	Deaths         int    `json:"deaths"`
	Assists        int    `json:"assists"`
	Win            bool   `json:"win"`
}
type Match struct {
	Metadata struct {
		MatchID      string   `json:"matchId"`
		Participants []string `json:"participants"`
	} `json:"metadata"`
	Info struct {
		GameID           uint64        `json:"gameId"`
		GameVersion      string        `json:"gameVersion"`
		QueueID          int           `json:"queueId"`
		GameDuration     uint64        `json:"gameDuration"`
		GameEndTimestamp int64         `json:"gameEndTimestamp"`
		Participants     []Participant `json:"participants"`
	} `json:"info"`
}

// MatchIDs uses the same regional host as Account-v1 (Asia by default), rather
// than the platform-specific Spectator host. Count must be between 1 and 100.
func (c *Client) MatchIDs(ctx context.Context, puuid string, start, count int) ([]string, error) {
	if err := checkContextAndID(ctx, puuid, "puuid"); err != nil {
		return nil, err
	}
	if start < 0 || count < 1 || count > 100 {
		return nil, errors.New("match start must be nonnegative and count between 1 and 100")
	}
	route := fmt.Sprintf("/lol/match/v5/matches/by-puuid/%s/ids?start=%d&count=%d", url.PathEscape(puuid), start, count)
	var ids []string
	if err := c.get(ctx, c.accountBaseURL, route, &ids); err != nil {
		return nil, err
	}
	if ids == nil {
		return nil, ErrInvalidResponse
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return nil, ErrInvalidResponse
		}
	}
	return ids, nil
}
func (c *Client) Match(ctx context.Context, matchID string) (Match, error) {
	if err := checkContextAndID(ctx, matchID, "matchID"); err != nil {
		return Match{}, err
	}
	var result Match
	if err := c.get(ctx, c.accountBaseURL, "/lol/match/v5/matches/"+url.PathEscape(matchID), &result); err != nil {
		return Match{}, err
	}
	if result.Metadata.MatchID != matchID || result.Info.GameID == 0 || result.Info.GameVersion == "" || len(result.Info.Participants) == 0 {
		return Match{}, ErrInvalidResponse
	}
	return result, nil
}
func (c *Client) Close() { c.httpClient.CloseIdleConnections() }
