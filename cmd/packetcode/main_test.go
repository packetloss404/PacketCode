package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/packetcode/packetcode/internal/jobs"
)

func TestWarnUnreadableJobRecordsSilentWhenNone(t *testing.T) {
	var buf bytes.Buffer
	warnUnreadableJobRecords(&buf, nil)
	if buf.Len() != 0 {
		t.Fatalf("warning emitted for zero records: %q", buf.String())
	}
}

func TestWarnUnreadableJobRecordsNamesPathAndReason(t *testing.T) {
	var buf bytes.Buffer
	warnUnreadableJobRecords(&buf, []jobs.UnreadableRecord{
		{Path: "/state/jobs/job_a.json", Reason: `unrecognised job state "abandoned"`},
	})
	out := buf.String()
	for _, want := range []string{
		"packetcode: 1 job record(s) were not loaded",
		"still on disk",
		"/state/jobs/job_a.json",
		`unrecognised job state "abandoned"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("warning missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "more not listed") {
		t.Fatalf("unexpected overflow line for a single record:\n%s", out)
	}
}

func TestWarnUnreadableJobRecordsBoundsListedEntries(t *testing.T) {
	records := make([]jobs.UnreadableRecord, 0, 12)
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		records = append(records, jobs.UnreadableRecord{
			Path:   "/state/jobs/job_" + id + ".json",
			Reason: "job record version 2 is newer than this build supports (1)",
		})
	}
	var buf bytes.Buffer
	warnUnreadableJobRecords(&buf, records)
	out := buf.String()

	if !strings.Contains(out, "packetcode: 12 job record(s) were not loaded") {
		t.Fatalf("warning does not state the full count:\n%s", out)
	}
	listed := strings.Count(out, "/state/jobs/job_")
	if listed != unreadableJobWarningLimit {
		t.Fatalf("listed %d records, want %d:\n%s", listed, unreadableJobWarningLimit, out)
	}
	if !strings.Contains(out, "... and 9 more not listed") {
		t.Fatalf("warning does not report the unlisted remainder:\n%s", out)
	}
	if lines := strings.Count(out, "\n"); lines != unreadableJobWarningLimit+2 {
		t.Fatalf("warning is %d lines, want %d:\n%s", lines, unreadableJobWarningLimit+2, out)
	}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if !strings.HasPrefix(line, "packetcode:") {
			t.Fatalf("line %q does not use the packetcode: stderr prefix", line)
		}
	}
}
