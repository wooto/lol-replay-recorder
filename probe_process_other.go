//go:build !windows

package recorder

import "os/exec"

func configureProbeProcess(*exec.Cmd) {}
