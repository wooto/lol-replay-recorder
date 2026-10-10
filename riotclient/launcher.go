package riotclient

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

var launchID = regexp.MustCompile(`^[a-z0-9_]+$`)

// LaunchProduct starts the explicitly supplied RiotClientServices executable
// using its product/patchline CLI. Call Wait on the returned command. Canceling
// ctx terminates that launcher process only, not a detached League game/client.
// Product launch is a CLI capability: no unsupported HTTP route is invented.
func LaunchProduct(ctx context.Context, executable, product, patchline string) (*exec.Cmd, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(executable) || !launchID.MatchString(product) || !launchID.MatchString(patchline) {
		return nil, errors.New("absolute executable and valid product/patchline IDs required")
	}
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("Riot Client executable unavailable")
	}
	cmd := exec.CommandContext(ctx, executable, "--launch-product="+product, "--launch-patchline="+patchline)
	if err = cmd.Start(); err != nil {
		return nil, errors.New("Riot Client product launch failed")
	}
	return cmd, nil
}
