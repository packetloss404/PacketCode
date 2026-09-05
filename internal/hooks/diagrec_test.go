package hooks

import (
	"fmt"
	"os"
	"sync"
)

// DIAGNOSTIC: go test discards a passing package's stderr, so timings written
// there are invisible on exactly the runs that matter. Appending to a file the
// workflow cats afterwards survives both a pass and a fail.
var diagMu sync.Mutex

func diagRecord(format string, args ...any) {
	path := os.Getenv("PACKETCODE_DIAG_FILE")
	if path == "" {
		return
	}
	diagMu.Lock()
	defer diagMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, format+"\n", args...)
}
