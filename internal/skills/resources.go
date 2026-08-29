package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Resource support exists because a skill body is frequently a dispatcher
// rather than a document.
//
// The Agent Skills convention that Claude Code, Cursor, and Codex all read
// puts the method in files beside SKILL.md -- references/, categories/,
// templates/ -- and leaves the body naming them for the agent to read when a
// task actually needs one. A loader that reads SKILL.md and stops therefore
// gets the table of contents and none of the book, which is worse than not
// loading the skill at all: the body confidently describes checklists that
// nothing can retrieve.
//
// These reads deliberately do not go through the read_file tool. That tool
// sandboxes to the project root via computers.RuntimeBackend, which is also
// the SSH path to a remote computer; widening it so a user-scope skill under
// ~/.packetcode/skills became reachable would widen every backend to solve a
// problem local to skills. Serving resources from the registry keeps the
// filesystem sandbox exactly where it is and confines these reads to a
// directory the registry itself resolved.
const (
	// MaxResourceBytes bounds one resource file. It matches MaxBodyBytes for
	// the same reason: a resource is paid for only when the model asks for it
	// by name, which is what buys the headroom the always-present index cannot.
	MaxResourceBytes = MaxBodyBytes

	// MaxResourcesListed bounds one listing. Published skills carry up to a
	// couple of hundred resource files, and the listing is rendered into a
	// turn; without a cap a skill directory becomes an unbounded write into
	// the context window.
	MaxResourcesListed = 200

	// maxResourceDepth bounds how deep a listing walk descends. It stops a
	// pathological tree from costing an unbounded walk to produce a listing
	// that is capped anyway.
	maxResourceDepth = 8
)

// Resources lists the files a skill carries beside its body, as slash-separated
// paths relative to the skill directory, in sorted order.
//
// The second return reports whether the listing was truncated at
// MaxResourcesListed. Truncation is reported rather than hidden so the caller
// can say so: a silently short listing tells the model a file does not exist
// when it does, which is the one failure mode worse than no listing.
func (r *Registry) Resources(name string) (list []string, truncated bool) {
	skill, ok := r.Lookup(name)
	if !ok || skill.Dir == "" {
		return nil, false
	}
	if skill.Source == SourceBuiltin {
		list, truncated = listEmbeddedResources(skill.Dir)
	} else {
		list, truncated = listDiskResources(skill.Dir)
	}
	sort.Strings(list)
	return list, truncated
}

// ReadResource returns one file from beside a skill's body.
//
// rel is model-supplied and is treated as such: it is confined to the skill
// directory both lexically and after symlink resolution, so neither "../.."
// nor a symlink planted inside a skill directory can turn a resource read into
// an arbitrary file read.
func (r *Registry) ReadResource(name, rel string) ([]byte, error) {
	skill, ok := r.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown skill %q", name)
	}
	if skill.Dir == "" {
		return nil, fmt.Errorf("skill %q carries no resource files", name)
	}
	cleaned, err := cleanResourceRel(rel)
	if err != nil {
		return nil, err
	}
	if cleaned == FileName {
		// The body is what the no-argument call already returns. Serving it
		// again here would just be a second, differently-labelled copy.
		return nil, fmt.Errorf("%s is the skill body; load it by calling the skill without a file", FileName)
	}
	if skill.Source == SourceBuiltin {
		return readEmbeddedResource(skill.Dir, cleaned)
	}
	return readDiskResource(skill.Dir, cleaned)
}

// cleanResourceRel validates a model-supplied relative path.
//
// It rejects rather than sanitises. Silently rewriting "../../etc/passwd" into
// something safe would hand back a file the caller did not ask for under a
// name it did not use, and the model would reason about the wrong content.
func cleanResourceRel(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("file is required")
	}
	if strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("invalid file path")
	}
	// Backslashes are accepted as separators because a model that has just
	// read a Windows path will offer one, but they are normalised before any
	// check runs so the checks below cannot be evaded by separator choice.
	rel = strings.ReplaceAll(rel, "\\", "/")
	// VolumeName catches "C:" and UNC prefixes, which are absolute on Windows
	// and are not caught by a leading-slash test.
	if strings.HasPrefix(rel, "/") || filepath.VolumeName(filepath.FromSlash(rel)) != "" {
		return "", fmt.Errorf("file must be relative to the skill directory")
	}
	cleaned := path.Clean(rel)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("file must stay inside the skill directory")
	}
	return cleaned, nil
}

func readDiskResource(dir, cleaned string) ([]byte, error) {
	target := filepath.Join(dir, filepath.FromSlash(cleaned))
	// Both sides are resolved before comparison. Resolving only the target
	// would compare a real path against a possibly-symlinked root and reject
	// legitimate reads; resolving neither would miss a symlink planted inside
	// the skill directory, which is the case that matters.
	dirReal, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve skill directory: %w", err)
	}
	targetReal, err := filepath.EvalSymlinks(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no such file in this skill: %s", cleaned)
		}
		return nil, err
	}
	if !withinDir(dirReal, targetReal) {
		return nil, fmt.Errorf("file resolves outside the skill directory: %s", cleaned)
	}
	info, err := os.Lstat(targetReal)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		// A device or FIFO stats as 0 bytes, passes any cap, and then reads
		// forever. loadSkillDir refuses one for the body; so does this.
		return nil, fmt.Errorf("not a regular file: %s", cleaned)
	}
	return readCapped(targetReal, MaxResourceBytes)
}

func readEmbeddedResource(dir, cleaned string) ([]byte, error) {
	data, err := fs.ReadFile(builtinFS, path.Join(dir, cleaned))
	if err != nil {
		return nil, fmt.Errorf("no such file in this skill: %s", cleaned)
	}
	if len(data) > MaxResourceBytes {
		return nil, fmt.Errorf("file is over the %d byte cap", MaxResourceBytes)
	}
	return data, nil
}

func listDiskResources(dir string) ([]string, bool) {
	var out []string
	truncated := false
	// WalkDir does not follow symlinked directories, which is what is wanted:
	// a listing that traversed a symlink would advertise paths that
	// readDiskResource is then correct to refuse.
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable subtree contributes nothing rather than aborting
			// the listing; a partial listing beats none.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if skipResourceName(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if strings.Count(rel, "/") >= maxResourceDepth {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if rel == FileName {
			return nil
		}
		if len(out) >= MaxResourcesListed {
			truncated = true
			return filepath.SkipAll
		}
		out = append(out, rel)
		return nil
	})
	return out, truncated
}

func listEmbeddedResources(dir string) ([]string, bool) {
	var out []string
	truncated := false
	_ = fs.WalkDir(builtinFS, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, dir), "/")
		if rel == "" || rel == FileName || skipResourceName(d.Name()) {
			return nil
		}
		if len(out) >= MaxResourcesListed {
			truncated = true
			return fs.SkipAll
		}
		out = append(out, rel)
		return nil
	})
	return out, truncated
}

// skipResourceName filters entries that are editor and VCS droppings rather
// than material a skill meant to publish.
func skipResourceName(name string) bool {
	return strings.HasPrefix(name, ".")
}

// withinDir reports whether target is root or sits beneath it. Both arguments
// must already be resolved; this is a lexical test on resolved paths, not a
// substitute for resolving them.
func withinDir(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
