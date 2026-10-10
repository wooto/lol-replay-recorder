//go:build !windows

package lcu

import (
	recorder "github.com/wooto/lol-replay-recorder"
)

func gameProcesses() (map[int]struct{}, error) { return nil, recorder.ErrUnsupportedPlatform }
func openGameProcess(int) (recorder.ReplayProcess, error) {
	return nil, recorder.ErrUnsupportedPlatform
}
