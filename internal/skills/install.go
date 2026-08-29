package skills

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Installation exists because the skills worth having are published as git
// repositories, and the alternative is a paragraph of documentation asking the
// user to clone one and copy directories by hand into a path they have to be
// told. That instruction is followed wrongly often enough -- copying SKILL.md
// alone, and leaving behind the references/ tree that carries the actual
// method -- that supporting it in the tool is cheaper than supporting the
// people who followed it approximately.
//
// Nothing here executes repository content. A clone is a download; the copy
// step reads and writes files. A skill body only ever reaches a model through
// the registry, labelled with the scope it came from.
const (
	// MaxInstallFiles bounds one installed skill. Large published skills carry
	// a few hundred files; this leaves room without letting a hostile or
	// mistaken repository write an unbounded tree into the user's home.
	MaxInstallFiles = 2000

	// MaxInstallBytes bounds one installed skill on disk.
	MaxInstallBytes = 32 * 1024 * 1024
)

// ScopeUser and ScopeProject name the two writable install destinations.
const (
	ScopeUser    = "user"
	ScopeProject = "project"
)

// InstallOptions describes one install request.
type InstallOptions struct {
	// Repo is "owner/repo", a full git URL, or a local path to a checkout.
	Repo string
	// Ref is an optional branch or tag.
	Ref string
	// Names limits the install to these skill directories. Empty installs all.
	Names []string
	// Scope is ScopeUser or ScopeProject.
	Scope string
	// WorkingDir locates the project scope. Ignored for the user scope.
	WorkingDir string
	// Force overwrites a skill directory that already exists.
	Force bool
}

// InstallResult reports what an install did.
type InstallResult struct {
	Dest      string
	Installed []string
	Replaced  []string
	// Skipped names skills already present that Force was not set for. They
	// are reported rather than treated as failure: installing eleven of twelve
	// skills and saying so is more useful than refusing the batch.
	Skipped []string
	// Rejected names directories that looked like skills but did not load,
	// with the reason. Validation happens before anything is copied, so a
	// malformed skill never reaches the user's skills directory at all.
	Rejected []string
}

var repoShorthand = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Install fetches a repository and copies its skills into the chosen scope.
//
// progress, when non-nil, receives one human-readable line per step.
func Install(opts InstallOptions, progress io.Writer) (InstallResult, error) {
	var result InstallResult

	dest, err := scopeDir(opts.Scope, opts.WorkingDir)
	if err != nil {
		return result, err
	}
	result.Dest = dest

	src, srcName, cleanup, err := fetchRepo(opts.Repo, opts.Ref, progress)
	if err != nil {
		return result, err
	}
	defer cleanup()

	found, err := discoverSkills(src, srcName)
	if err != nil {
		return result, err
	}
	if len(found) == 0 {
		return result, fmt.Errorf("no skills found in %s: expected <repo>/skills/<name>/%s, <repo>/<name>/%s, or a %s at the repository root",
			opts.Repo, FileName, FileName, FileName)
	}
	found, err = selectSkills(found, opts.Names)
	if err != nil {
		return result, err
	}

	if err := os.MkdirAll(dest, 0o700); err != nil {
		return result, fmt.Errorf("create %s: %w", dest, err)
	}

	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		srcDir := found[name]
		// Validated before it is copied, not after. A skill that fails to load
		// is worth nothing on disk, and one that has already been written is a
		// mess the user has to clean up by hand to get back to where they were.
		if err := validateSkillDir(srcDir, name); err != nil {
			result.Rejected = append(result.Rejected, fmt.Sprintf("%s: %s", name, err))
			continue
		}
		target := filepath.Join(dest, name)
		existed := false
		if _, err := os.Stat(target); err == nil {
			if !opts.Force {
				result.Skipped = append(result.Skipped, name)
				continue
			}
			existed = true
		}
		if err := replaceDir(srcDir, target); err != nil {
			return result, fmt.Errorf("install %s: %w", name, err)
		}
		if existed {
			result.Replaced = append(result.Replaced, name)
		} else {
			result.Installed = append(result.Installed, name)
		}
		if progress != nil {
			verb := "installed"
			if existed {
				verb = "updated"
			}
			fmt.Fprintf(progress, "  %s %s\n", verb, name)
		}
	}
	return result, nil
}

// Remove deletes one installed skill directory.
func Remove(name, scope, workingDir string) (string, error) {
	if !ValidName(name) {
		return "", fmt.Errorf("invalid skill name %q", name)
	}
	dest, err := scopeDir(scope, workingDir)
	if err != nil {
		return "", err
	}
	target := filepath.Join(dest, name)
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%s is not installed in the %s scope", name, scope)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a skill directory", target)
	}
	if err := os.RemoveAll(target); err != nil {
		return "", err
	}
	return target, nil
}

func scopeDir(scope, workingDir string) (string, error) {
	switch scope {
	case ScopeUser, "":
		return UserSkillsDir()
	case ScopeProject:
		if strings.TrimSpace(workingDir) == "" {
			return "", fmt.Errorf("the project scope needs a working directory")
		}
		return ProjectSkillsDir(workingDir), nil
	default:
		return "", fmt.Errorf("unknown scope %q: use %q or %q", scope, ScopeUser, ScopeProject)
	}
}

// fetchRepo resolves a repo argument to a local directory to read skills from,
// and reports the name a repository that is itself one skill should take.
//
// The name has to come from here. A remote clone lands in an os.MkdirTemp
// directory, so deriving it at the discovery end from the directory basename
// named the skill after the temporary directory -- and "packetcode-skills-1642398117"
// is a valid skill name, so it installed cleanly under that garbage rather
// than failing where somebody would notice.
func fetchRepo(repo, ref string, progress io.Writer) (dir, name string, cleanup func(), err error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "", "", func() {}, fmt.Errorf("a repository is required")
	}
	// A local checkout is read in place. This is what makes the command
	// testable without a network, and it is also the honest way to install a
	// skill the user is writing themselves.
	if info, statErr := os.Stat(repo); statErr == nil && info.IsDir() {
		abs, absErr := filepath.Abs(repo)
		if absErr != nil {
			return "", "", func() {}, absErr
		}
		return abs, filepath.Base(abs), func() {}, nil
	}

	url := repo
	if repoShorthand.MatchString(repo) {
		url = "https://github.com/" + repo
	}
	if _, lookErr := exec.LookPath("git"); lookErr != nil {
		return "", "", func() {}, fmt.Errorf("git is required to install from %s but was not found on PATH", url)
	}

	tmp, err := os.MkdirTemp("", "packetcode-skills-")
	if err != nil {
		return "", "", func() {}, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }

	args := []string{"clone", "--depth", "1", "--single-branch"}
	if strings.TrimSpace(ref) != "" {
		args = append(args, "--branch", ref)
	}
	// "--" terminates options so a repository argument beginning with a dash
	// is a bad URL rather than a git flag.
	args = append(args, "--", url, tmp)

	if progress != nil {
		fmt.Fprintf(progress, "cloning %s\n", url)
	}
	cmd := exec.Command("git", args...)
	// Any credential prompt would block a non-interactive command forever.
	// Failing with git's own error is the better outcome.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "GCM_INTERACTIVE=never")
	out, err := cmd.CombinedOutput()
	if err != nil {
		cleanup()
		return "", "", func() {}, fmt.Errorf("git clone %s: %w\n%s", url, err, strings.TrimSpace(string(out)))
	}
	return tmp, repoBaseName(url), cleanup, nil
}

// repoBaseName reduces a git URL to the repository name, which is the name a
// repository that is itself one skill is published under.
func repoBaseName(url string) string {
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	url = strings.TrimSuffix(url, ".git")
	// Both separators matter: scp-style remotes ("git@host:owner/repo") put the
	// owner after a colon rather than a slash.
	if i := strings.LastIndexAny(url, "/:"); i >= 0 {
		url = url[i+1:]
	}
	return url
}

// discoverSkills finds skill directories in a checkout, keyed by skill name.
//
// It accepts the three layouts published repositories actually use rather than
// insisting on one: a marketplace repo of many skills under skills/, the same
// laid out at the root, and a repository that is itself a single skill.
func discoverSkills(root, repoName string) (map[string]string, error) {
	out := map[string]string{}

	if _, err := os.Stat(filepath.Join(root, FileName)); err == nil {
		// The name comes from the repository, never from the directory holding
		// the checkout: for a remote install that directory is a temporary one,
		// and its basename is a valid skill name, so taking it installed the
		// skill under "packetcode-skills-1642398117" without failing anywhere
		// the user would look.
		name := sanitiseName(repoName)
		if name == "" {
			return nil, fmt.Errorf("this repository is a single skill, but %q is not usable as a skill name: use only letters, digits, '-' and '_'", repoName)
		}
		out[name] = root
		return out, nil
	}

	for _, container := range []string{filepath.Join(root, "skills"), root} {
		entries, err := os.ReadDir(container)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			dir := filepath.Join(container, entry.Name())
			if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
				continue
			}
			name := sanitiseName(entry.Name())
			if name == "" {
				continue
			}
			out[name] = dir
		}
		if len(out) > 0 {
			// A skills/ directory wins outright. Falling through to the root
			// after finding one would sweep up the repository's own tooling
			// directories on any repo that happens to keep a SKILL.md in them.
			return out, nil
		}
	}
	return out, nil
}

// sanitiseName returns the directory name if it is usable as a skill name.
// It does not repair one: a name is the model's handle for the skill and the
// user's handle for the directory, and quietly renaming it would break both.
func sanitiseName(name string) string {
	if ValidName(name) {
		return name
	}
	return ""
}

func selectSkills(found map[string]string, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return found, nil
	}
	out := map[string]string{}
	var missing []string
	for _, want := range names {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		dir, ok := found[want]
		if !ok {
			missing = append(missing, want)
			continue
		}
		out[want] = dir
	}
	if len(missing) > 0 {
		available := make([]string, 0, len(found))
		for name := range found {
			available = append(available, name)
		}
		sort.Strings(available)
		return nil, fmt.Errorf("not in this repository: %s\navailable: %s",
			strings.Join(missing, ", "), strings.Join(available, ", "))
	}
	return out, nil
}

// validateSkillDir loads a candidate the way discovery will, so a skill that
// cannot be used is refused at install time with a reason rather than sitting
// in the user's skills directory contributing a line to Errors() forever.
func validateSkillDir(dir, name string) error {
	_, err := loadSkillDir(dir, name, SourceUser)
	return err
}

// replaceDir copies src over dst atomically enough that a failure part-way
// leaves the previous install in place rather than a half-written directory.
func replaceDir(src, dst string) error {
	staging := dst + ".packetcode-partial"
	_ = os.RemoveAll(staging)
	if err := copyTree(src, staging); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}
	backup := ""
	if _, err := os.Stat(dst); err == nil {
		backup = dst + ".packetcode-old"
		_ = os.RemoveAll(backup)
		if err := os.Rename(dst, backup); err != nil {
			_ = os.RemoveAll(staging)
			return err
		}
	}
	if err := os.Rename(staging, dst); err != nil {
		if backup != "" {
			_ = os.Rename(backup, dst)
		}
		_ = os.RemoveAll(staging)
		return err
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return nil
}

// copyTree copies a skill directory, refusing anything that is not a plain
// file or directory.
//
// Symlinks are skipped rather than recreated. A skill is reference material,
// and a link is the one thing in such a tree that can point somewhere the user
// did not agree to -- reproducing one inside ~/.packetcode/skills would plant
// exactly the escape that ReadResource then has to refuse.
func copyTree(src, dst string) error {
	files, bytes := 0, int64(0)
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		// The repository's own git metadata is not part of the skill.
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return fs.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files++
		bytes += info.Size()
		if files > MaxInstallFiles {
			return fmt.Errorf("skill has more than %d files", MaxInstallFiles)
		}
		if bytes > MaxInstallBytes {
			return fmt.Errorf("skill is larger than %d bytes", MaxInstallBytes)
		}
		return copyFile(p, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// Executable bits are preserved for the scripts published skills carry,
	// but nothing here runs them; that stays a decision for execute_command
	// and its approval.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
