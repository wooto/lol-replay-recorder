//go:build !windows || !amd64

package recorder

import "context"

type nativeDesktop struct{}

func (nativeDesktop) acquire() (func(), error) { return nil, ErrUnsupportedPlatform }
func (nativeDesktop) launch(context.Context, Config, string) (replayProcess, error) {
	return nil, ErrUnsupportedPlatform
}
func (nativeDesktop) selectPlayer(context.Context, int, uint16) error { return ErrUnsupportedPlatform }
