//go:build !windows

package extensions

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
