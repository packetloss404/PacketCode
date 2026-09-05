//go:build !windows

package atomicfile

// isShareViolation is always false away from Windows.
//
// POSIX renames succeed with the destination open, and a reader holding an
// unlinked inode keeps reading it, so there is no transient state to retry
// through. The retry loops below therefore run exactly once here: same syscall
// count, same behaviour, no sleep.
func isShareViolation(error) bool { return false }
