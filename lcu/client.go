// Package lcu integrates the signed-in League Client's local API.
package lcu

import (
	"context"

	"github.com/wooto/lol-replay-recorder/internal/localhttp"
	"github.com/wooto/lol-replay-recorder/localclient"
)

type Config = localhttp.Config
type StatusError = localhttp.StatusError
type Client struct{ http *localhttp.Client }
type Configuration struct {
	GameVersion      string `json:"gameVersion"`
	IsLoggedIn       bool   `json:"isLoggedIn"`
	IsPatching       bool   `json:"isPatching"`
	IsPlayingGame    bool   `json:"isPlayingGame"`
	IsPlayingReplay  bool   `json:"isPlayingReplay"`
	IsReplaysEnabled bool   `json:"isReplaysEnabled"`
}

func New(config Config) (*Client, error) {
	h, err := localhttp.New(config, "", true)
	if err != nil {
		return nil, err
	}
	return &Client{http: h}, nil
}
func (c *Client) Close() { c.http.Close() }
func FromLockfile(path string) (*Client, error) {
	credentials, err := localclient.ReadLockfile(path)
	if err != nil {
		return nil, err
	}
	return New(Config{BaseURL: credentials.BaseURL(), Token: credentials.Token})
}
func (c *Client) Configuration(ctx context.Context) (Configuration, error) {
	var result Configuration
	err := c.http.Request(ctx, "GET", "/lol-replays/v1/configuration", nil, &result)
	return result, err
}
