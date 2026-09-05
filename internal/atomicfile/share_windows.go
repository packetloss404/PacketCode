//go:build windows

package atomicfile

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// isShareViolation reports whether err is Windows refusing an operation only
// because someone else has the file open at this instant.
//
// Windows opens deny by default. Go's os.ReadFile asks for FILE_SHARE_READ and
// FILE_SHARE_WRITE but not FILE_SHARE_DELETE, so while any reader holds a
// handle, a rename onto that path fails with ERROR_ACCESS_DENIED -- and while a
// rename is replacing the file, a reader fails with ERROR_SHARING_VIOLATION.
// Neither says anything is wrong with the file or the caller; both mean "try
// again in a moment", which is what POSIX does implicitly by allowing the
// rename to proceed under an open handle.
//
// Measured on this repository's own job records: with one reader and one writer
// contending on a single path, 809 of 2000 renames failed with errno 5 and 857
// reads failed with errno 32. Treating those as permanent is what let a job
// record vanish and be reported as malformed.
func isShareViolation(err error) bool {
	if err == nil {
		return false
	}
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == syscall.Errno(windows.ERROR_ACCESS_DENIED) ||
		errno == syscall.Errno(windows.ERROR_SHARING_VIOLATION)
}
