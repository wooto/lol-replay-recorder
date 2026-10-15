package recorder

import (
	"os/exec"
	"syscall"
)

func configureProbeProcess(cmd *exec.Cmd) {
	// CREATE_NO_WINDOW prevents a console from stealing replay focus. Hiding
	// an already-created window alone does not prevent a transient console.
	const createNoWindow = 0x08000000
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
