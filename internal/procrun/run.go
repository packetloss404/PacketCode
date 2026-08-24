package procrun

import (
	"os/exec"
	"time"
)

func ConfigureTreeCancel(cmd *exec.Cmd) {
	configurePlatform(cmd)
	cmd.Cancel = func() error {
		return KillTree(cmd)
	}
	cmd.WaitDelay = 250 * time.Millisecond
}

// ConfigureTrackedTreeCancel is the strict subprocess variant. On Windows it
// creates the process suspended so TrackTree can atomically assign a Job Object
// before any child code can run; TrackTree then resumes it.
func ConfigureTrackedTreeCancel(cmd *exec.Cmd) {
	configureTrackedPlatform(cmd)
	cmd.Cancel = func() error {
		return KillTree(cmd)
	}
	cmd.WaitDelay = 250 * time.Millisecond
}

// TrackTree attaches platform-specific lifetime tracking after Start. On
// Windows this places the process in a kill-on-close Job Object so descendants
// cannot survive a normally exiting parent.
func TrackTree(cmd *exec.Cmd) error {
	return trackTree(cmd)
}

// ReleaseTree releases platform tracking after Wait. On Windows, closing the
// Job Object also terminates any descendants left behind by the root process.
func ReleaseTree(cmd *exec.Cmd) error {
	return releaseTree(cmd)
}
