// Package riotclient integrates the already signed-in Riot Client remoting API.
// Credentials belong to Riot Client, independently of LCU or Riot Web API keys.
package riotclient

import (
	"context"

	"github.com/wooto/lol-replay-recorder/internal/localhttp"
	"github.com/wooto/lol-replay-recorder/localclient"
)

type Config = localhttp.Config
type StatusError = localhttp.StatusError
type Client struct{ http *localhttp.Client }
type Authorization struct {
	Subject           string `json:"subject"`
	CurrentPlatformID string `json:"currentPlatformId"`
	CurrentAccountID  uint64 `json:"currentAccountId"`
}
type Session struct {
	ProductID           string `json:"productId"`
	PatchlineID         string `json:"patchlineId"`
	Phase               string `json:"phase"`
	Version             string `json:"version"`
	LaunchConfiguration struct {
		Executable string `json:"executable"`
	} `json:"launchConfiguration"`
}
type RegionLocale struct {
	Region string `json:"region"`
	Locale string `json:"locale"`
}

func (c *Client) Sessions(ctx context.Context) (map[string]Session, error) {
	var sessions map[string]Session
	err := c.http.Request(ctx, "GET", "/product-session/v1/sessions", nil, &sessions)
	return sessions, err
}
func (c *Client) Region(ctx context.Context) (RegionLocale, error) {
	var region RegionLocale
	err := c.http.Request(ctx, "GET", "/riotclient/region-locale", nil, &region)
	return region, err
}
func New(config Config) (*Client, error) {
	h, err := localhttp.New(config, "", true)
	if err != nil {
		return nil, err
	}
	return &Client{http: h}, nil
}
func (c *Client) Close() { c.http.Close() }
func FromCommandLine(line string) (*Client, error) {
	credentials, err := localclient.FromCommandLine(line, true)
	if err != nil {
		return nil, err
	}
	return New(Config{BaseURL: credentials.BaseURL(), Token: credentials.Token})
}
func (c *Client) Authorization(ctx context.Context) (Authorization, error) {
	var auth Authorization
	err := c.http.Request(ctx, "GET", "/rso-auth/v1/authorization", nil, &auth)
	return auth, err
}
