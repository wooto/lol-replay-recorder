package lcu

import (
	"context"
	"errors"
	"strings"
)

// SpectateRequest is an LCU handoff, not an observer archive or .rofl.
// SpectatorKey must come from a supported client flow; it is not assumed to be
// interchangeable with Spectator-v5's observers.encryptionKey.
type SpectateRequest struct {
	PUUID        string `json:"puuid"`
	SpectatorKey string `json:"spectatorKey"`
}

func (s SpectateRequest) String() string   { return "spectator handoff (credentials redacted)" }
func (s SpectateRequest) GoString() string { return s.String() }
func (c *Client) Spectate(ctx context.Context, request SpectateRequest) error {
	if strings.TrimSpace(request.PUUID) == "" || strings.TrimSpace(request.SpectatorKey) == "" {
		return errors.New("PUUID and spectator key are required")
	}
	if err := c.idle(ctx); err != nil {
		return err
	}
	return c.http.Request(ctx, "POST", "/lol-spectator/v1/spectate/launch", request, nil)
}
