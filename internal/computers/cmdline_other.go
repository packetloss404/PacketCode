//go:build !windows

package computers

import "os/exec"

// applyShellCommandLine is a no-op off Windows: `sh -c` receives the command
// as one argv entry and needs no re-quoting.
func applyShellCommandLine(*exec.Cmd, string) {}
