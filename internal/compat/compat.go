// Package compat is the single place that says what packetcode writes to
// disk, what version each format is at, and what happens when a build meets a
// file it was not built for.
//
// It exists because that answer was previously distributed. Five packages each
// declared their own version constant and each decided independently what to do
// about a mismatch, and they did not agree: jobs refused a newer record,
// computers refused a newer registry, skills refused anything but an exact
// match, workflows refused anything but an exact match *including older ones*,
// sessions carried a version and enforced nothing at all, and config had no
// version to enforce. A user cannot reason about a guarantee spelled six
// different ways, and neither can we.
//
// The constants live here rather than in the packages that use them so that one
// file is the contract. docs/compatibility.md is the published form of this
// file, and compat_doc_test.go fails when the two disagree -- a contract that
// can drift from the code silently is not a contract, it is a document.
package compat

import "fmt"

// Versions of every format packetcode persists.
//
// Bumping one of these is a compatibility event: see docs/compatibility.md for
// what has to happen alongside it. In particular a bump is only correct when
// the change is one an older build would get *wrong*; a purely additive field
// that older builds can ignore does not need one, and spending a version on it
// makes every older build refuse a file it could have read.
const (
	// ConfigVersion covers ~/.packetcode/config.toml, which includes MCP
	// server definitions under [mcp.<name>] -- they are not a separate format.
	ConfigVersion = 1
	// SessionVersion covers ~/.packetcode/sessions/<id>.json. Version 2 added
	// the immutable model projection for oversized tool results.
	SessionVersion = 2
	// JobVersion covers ~/.packetcode/jobs/<id>.json.
	JobVersion = 1
	// ComputerRegistryVersion covers ~/.packetcode/computers.json.
	ComputerRegistryVersion = 1
	// SkillApprovalVersion covers ~/.packetcode/skill-approvals.json.
	SkillApprovalVersion = 1
	// WorkflowSchemaVersion covers workflow TOML files, which are authored by
	// hand rather than written by packetcode.
	WorkflowSchemaVersion = 1
)

// Rule is what a build does when the version on disk is not its own.
type Rule int

const (
	// ReadOlder accepts an older file, migrating it forward, and refuses a
	// newer one. The default for anything packetcode writes itself.
	//
	// Refusing the newer direction is the whole point. Go's decoders discard
	// fields they do not know, so an older build that reads a newer file and
	// then saves has not merely misread it -- it has destroyed the parts it
	// could not see, permanently, with no error anywhere.
	ReadOlder Rule = iota

	// ExactOnly accepts one version and refuses every other, in both
	// directions. Correct where there is nothing to lose by refusing and
	// something to lose by guessing: an approval store read wrongly would
	// grant authority nobody granted, so "refuse and treat nothing as
	// approved" is the safe reading of an unfamiliar file.
	ExactOnly

	// Advisory reports a mismatch and carries on. Correct only for files the
	// user writes by hand, where refusing to start is a worse outcome than
	// ignoring a setting -- provided the ignoring is said out loud.
	Advisory
)

func (r Rule) String() string {
	switch r {
	case ReadOlder:
		return "read older, refuse newer"
	case ExactOnly:
		return "exact version only"
	case Advisory:
		return "report and continue"
	default:
		return "unknown"
	}
}

// Format is one persisted on-disk format.
type Format struct {
	// Name is how the format is referred to in errors and in the published
	// contract. Stable: users grep for it.
	Name string
	// Location is where it lives, relative to the packetcode home unless it
	// says otherwise.
	Location string
	// Version is what this build writes.
	Version int
	// Rule is what this build does about a version that is not its own.
	Rule Rule
	// AuthoredByHand marks formats a person edits. It is the reason a format
	// may be Advisory: refusing to start over a config file someone typed is
	// a worse failure than the one being prevented.
	AuthoredByHand bool
}

// Formats returns every persisted format, in the order the contract lists
// them. The doc test walks this.
func Formats() []Format {
	return []Format{
		{
			Name:           "config",
			Location:       "config.toml",
			Version:        ConfigVersion,
			Rule:           Advisory,
			AuthoredByHand: true,
		},
		{
			Name:     "session",
			Location: "sessions/<id>.json",
			Version:  SessionVersion,
			Rule:     ReadOlder,
		},
		{
			Name:     "job",
			Location: "jobs/<id>.json",
			Version:  JobVersion,
			Rule:     ReadOlder,
		},
		{
			Name:     "computer registry",
			Location: "computers.json",
			Version:  ComputerRegistryVersion,
			Rule:     ReadOlder,
		},
		{
			Name:     "skill approvals",
			Location: "skill-approvals.json",
			Version:  SkillApprovalVersion,
			Rule:     ExactOnly,
		},
		{
			Name:           "workflow",
			Location:       "workflows/*.toml",
			Version:        WorkflowSchemaVersion,
			Rule:           ExactOnly,
			AuthoredByHand: true,
		},
	}
}

// TooNew is the error a reader returns for a record from a future build.
//
// One phrasing for every format, because this message is the entire user-facing
// surface of the policy. It has to say three things, and the versions alone say
// none of them: what was refused, that the refusal is deliberate, and what to
// do -- which is always the same thing, so it should always read the same way.
func TooNew(format string, found, supported int) error {
	return fmt.Errorf(
		"%s format version %d is newer than this build supports (%d); "+
			"refusing rather than reading it wrongly and saving over what it cannot see. "+
			"Upgrade packetcode, or move the file aside",
		format, found, supported)
}

// Unsupported is the error for a format that accepts exactly one version.
func Unsupported(format string, found, supported int) error {
	return fmt.Errorf(
		"%s format version %d is not supported by this build (expected %d)",
		format, found, supported)
}
