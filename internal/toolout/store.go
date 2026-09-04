// Package toolout keeps oversized tool output out of the model's context
// without losing it. The full bytes are spilled to a per-session directory on
// disk; the model receives a bounded excerpt plus an opaque handle it can read
// more from, so a cap stops meaning permanent loss.
//
// The handle is deliberately NOT a path and no path is ever derived from
// model-supplied text. Handles are random IDs minted here and looked up in this
// process's in-memory registry, whose values carry the only paths involved. A
// model that invents a handle — including one shaped like `../../../etc/passwd`
// — misses the map and gets "no longer retained", never a file it was not
// already shown. That is what keeps the read-more tool from becoming an
// arbitrary-file-read primitive that bypasses the tools' root confinement.
package toolout

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/packetcode/packetcode/internal/config"
)

const (
	// DefaultExcerptLimit bounds what one tool result contributes to the
	// model's context. It is under the session layer's 64 KiB projection cap so
	// the excerpt this package writes is what the model actually sees, rather
	// than being truncated a second time by a layer that knows nothing about
	// the handle.
	DefaultExcerptLimit = 32 * 1024
	// DefaultBudget bounds one session's spill directory. Without it a chatty
	// MCP server would grow ~/.packetcode/ without limit for the lifetime of a
	// long session.
	DefaultBudget = 64 << 20
	// DefaultMaxAge bounds how long an abandoned spill directory survives a
	// crash. Sessions clean up on Close; this is the backstop for the process
	// that never got to run it.
	DefaultMaxAge = 24 * time.Hour

	// DefaultPageBytes and MaxPageBytes bound one read-more call so retrieving
	// spilled output cannot itself blow the context budget the spill exists to
	// protect.
	DefaultPageBytes = 8 * 1024
	MaxPageBytes     = 32 * 1024

	// handlePrefix makes a handle recognisable in a transcript and lets an
	// obviously-not-a-handle argument be rejected before any lookup.
	handlePrefix = "out_"
	handleHexLen = 32

	spillDirPrefix = "s-"
	spillFileExt   = ".out"
)

// errTooLarge reports output bigger than the whole session budget: retaining it
// would mean evicting everything else and still overrunning, so it is excerpted
// without a handle instead.
var errTooLarge = errors.New("toolout: content exceeds session budget")

// Options tune a Store. The zero value selects the defaults above.
type Options struct {
	ExcerptLimit int
	Budget       int64
	MaxAge       time.Duration
}

func (o Options) withDefaults() Options {
	if o.ExcerptLimit <= 0 {
		o.ExcerptLimit = DefaultExcerptLimit
	}
	if o.Budget <= 0 {
		o.Budget = DefaultBudget
	}
	if o.MaxAge <= 0 {
		o.MaxAge = DefaultMaxAge
	}
	return o
}

type entry struct {
	path string
	size int64
	tool string
}

// Store is one session's bounded tool-output spill area.
//
// It is per-session state, not global, for the reason TodoStore is: foreground,
// ACP, and every background job run their own agent, and a handle minted by one
// must not be readable from another. Each agent gets its own Store, so a
// background job cannot read (or evict) the foreground session's output.
type Store struct {
	mu      sync.Mutex
	dir     string
	opts    Options
	entries map[string]*entry
	order   []string // insertion order; eviction is oldest-first
	used    int64
	closed  bool
}

// DefaultRoot returns <packetcode home>/tool-output, the parent of every
// session's spill directory. The directory is not created.
func DefaultRoot() (string, error) {
	home, err := config.ResolveHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "tool-output"), nil
}

// OpenDefault opens a store under DefaultRoot. It is the one call a runtime
// needs: pass the returned store to agent.Config.ToolOutput and to
// tools.NewReadToolOutputTool so the minting side and the reading side share
// one registry, and Close it when the session or job ends.
func OpenDefault(opts Options) (*Store, error) {
	root, err := DefaultRoot()
	if err != nil {
		return nil, err
	}
	return Open(root, opts)
}

// Open creates a fresh spill directory for one session under root and prunes
// stale siblings left by earlier runs. Pruning happens here — not only on
// Close — because a session that is killed never reaches Close, and packetcode
// already has one place where abandoned files accumulate forever; this must not
// become the second.
func Open(root string, opts Options) (*Store, error) {
	opts = opts.withDefaults()
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("toolout: empty root")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("toolout: create %s: %w", root, err)
	}
	Prune(root, opts.MaxAge)
	id, err := newID()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, spillDirPrefix+id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("toolout: create %s: %w", dir, err)
	}
	return &Store{dir: dir, opts: opts, entries: map[string]*entry{}}, nil
}

// Prune removes spill directories not modified within maxAge. It is safe to
// call at startup and ignores anything it does not recognise, so an unrelated
// file under root is never deleted.
func Prune(root string, maxAge time.Duration) {
	if maxAge <= 0 {
		maxAge = DefaultMaxAge
	}
	listing, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, item := range listing {
		if !item.IsDir() || !strings.HasPrefix(item.Name(), spillDirPrefix) {
			continue
		}
		info, err := item.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		// Best effort: a live session on another machine sharing the data home
		// keeps its directory's mtime fresh by spilling, and losing an old
		// spill only degrades a handle to "no longer retained".
		_ = os.RemoveAll(filepath.Join(root, item.Name()))
	}
}

// Dir reports the session's spill directory. Exposed for tests and diagnostics
// only; handles never travel as paths.
func (s *Store) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

// Capture returns the model-facing text for one tool result and whether it was
// replaced by a bounded excerpt.
//
// Output within the limit is returned unchanged and reports false, so ordinary
// results stay byte-identical to what the model saw before this package
// existed. Oversized output is written to disk and replaced by a head+tail
// excerpt that names the omitted range and the handle to retrieve it with. If
// the spill itself fails, the excerpt is still returned — bounded context
// matters more than retrieval, and the marker says the remainder is gone.
func (s *Store) Capture(toolName, content string) (string, bool) {
	if s == nil || len(content) <= s.limit() {
		return content, false
	}
	handle, err := s.spill(toolName, content)
	if err != nil {
		return Excerpt(content, s.limit(), ""), true
	}
	return Excerpt(content, s.limit(), handle), true
}

func (s *Store) limit() int {
	if s == nil || s.opts.ExcerptLimit <= 0 {
		return DefaultExcerptLimit
	}
	return s.opts.ExcerptLimit
}

func (s *Store) spill(toolName, content string) (string, error) {
	size := int64(len(content))
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", errors.New("toolout: store closed")
	}
	if size > s.opts.Budget {
		s.mu.Unlock()
		return "", errTooLarge
	}
	s.evictLocked(size)
	handle, err := newHandle()
	if err != nil {
		s.mu.Unlock()
		return "", err
	}
	path := filepath.Join(s.dir, handle+spillFileExt)
	s.mu.Unlock()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("toolout: write spill: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		_ = os.Remove(path)
		return "", errors.New("toolout: store closed")
	}
	s.entries[handle] = &entry{path: path, size: size, tool: toolName}
	s.order = append(s.order, handle)
	s.used += size
	return handle, nil
}

// evictLocked drops oldest-first until need fits in the budget. An evicted
// handle is removed from the registry, so a later reference degrades to "no
// longer retained" rather than reading a file that has been replaced.
func (s *Store) evictLocked(need int64) {
	for s.used+need > s.opts.Budget && len(s.order) > 0 {
		oldest := s.order[0]
		s.order = s.order[1:]
		if e, ok := s.entries[oldest]; ok {
			delete(s.entries, oldest)
			s.used -= e.size
			_ = os.Remove(e.path)
		}
	}
}

// Page is one bounded window of a spilled tool result.
type Page struct {
	Handle string
	Tool   string
	Text   string
	Offset int64 // byte offset of Text within the full output
	Next   int64 // offset to pass to the following call
	Total  int64
	EOF    bool
}

// Read returns at most limit bytes of the output behind handle, starting at
// offset. The second result is false for any handle this Store does not hold —
// malformed, invented, expired, evicted, or minted by another session. Callers
// must render that as an ordinary "no longer retained" result: the miss is the
// same for a pruned handle and for a path-traversal attempt, so nothing about
// the filesystem can be probed through it.
func (s *Store) Read(handle string, offset int64, limit int) (Page, bool) {
	if s == nil || !ValidHandle(handle) {
		return Page{}, false
	}
	s.mu.Lock()
	e, ok := s.entries[handle]
	s.mu.Unlock()
	if !ok {
		return Page{}, false
	}
	if limit <= 0 {
		limit = DefaultPageBytes
	}
	if limit > MaxPageBytes {
		limit = MaxPageBytes
	}
	if offset < 0 {
		offset = 0
	}

	f, err := os.Open(e.path)
	if err != nil {
		return Page{}, false
	}
	defer f.Close()

	page := Page{Handle: handle, Tool: e.tool, Offset: offset, Total: e.size}
	if offset >= e.size {
		page.Offset, page.Next, page.EOF = e.size, e.size, true
		return page, true
	}
	buf := make([]byte, limit)
	n, err := f.ReadAt(buf, offset)
	if n <= 0 && err != nil {
		return Page{}, false
	}
	buf = buf[:n]
	// Never hand the model a half rune: skip a leading continuation byte
	// (only possible when the model picked an offset mid-rune) and drop a
	// trailing rune the window cut in half.
	skipped := 0
	for len(buf) > 0 && !utf8.RuneStart(buf[0]) {
		buf = buf[1:]
		skipped++
	}
	end := offset + int64(skipped) + int64(len(buf))
	if end < e.size {
		buf = trimPartialRune(buf)
	}
	page.Offset = offset + int64(skipped)
	page.Text = string(buf)
	page.Next = page.Offset + int64(len(buf))
	page.EOF = page.Next >= e.size
	return page, true
}

// Close removes this session's spill directory. Safe to call more than once.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.entries = map[string]*entry{}
	s.order = nil
	s.used = 0
	if s.dir == "" {
		return nil
	}
	return os.RemoveAll(s.dir)
}

// ValidHandle reports whether text has the exact shape this package mints.
// Shape validation is a cheap filter, not the security boundary — the registry
// lookup is — but it means obviously hostile arguments never reach it.
func ValidHandle(text string) bool {
	if len(text) != len(handlePrefix)+handleHexLen {
		return false
	}
	if !strings.HasPrefix(text, handlePrefix) {
		return false
	}
	for _, r := range text[len(handlePrefix):] {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}

func newHandle() (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	return handlePrefix + id, nil
}

func newID() (string, error) {
	raw := make([]byte, handleHexLen/2)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("toolout: generate id: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func trimPartialRune(b []byte) []byte {
	for i := len(b) - 1; i >= 0 && i > len(b)-utf8.UTFMax-1; i-- {
		if !utf8.RuneStart(b[i]) {
			continue
		}
		if r, size := utf8.DecodeRune(b[i:]); r == utf8.RuneError && size <= 1 {
			return b[:i]
		}
		return b
	}
	return b
}
