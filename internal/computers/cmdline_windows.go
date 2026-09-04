//go:build windows

package computers

import "os/exec"

// applyShellCommandLine hands cmd.exe the command verbatim. os/exec builds
// the process command line by quoting each argument the way a C runtime
// would parse it, but cmd.exe does not parse like a C runtime: it strips the
// outer quotes of the /C argument and leaves the backslash-escaped inner ones
// in place. `cmd /S /C "<command>"` is the documented form that removes
// exactly the outer pair and passes everything inside through untouched.
func applyShellCommandLine(cmd *exec.Cmd, command string) {
	if cmd.SysProcAttr == nil {
		return
	}
	cmd.SysProcAttr.CmdLine = `cmd /S /C "` + command + `"`
}
