package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DotEnvFile is the filename read for provider keys at both scopes.
const DotEnvFile = ".env"

// maxDotEnvBytes bounds what will be read. A .env is a handful of lines; a
// hundred-megabyte one is a mistake or a hostile file, and either way reading
// it into memory to look for API keys helps nobody.
const maxDotEnvBytes = 256 * 1024

// DotEnv is the parsed contents of one or more .env files.
//
// Deliberately NOT injected into the process environment. packetcode runs
// shell commands on the model's behalf, and os.Setenv would hand every one of
// them -- plus every hook and MCP server -- the contents of the user's .env.
// A file that exists to hold API keys is the last thing that should be
// inherited by an arbitrary subprocess. It is consulted where keys are
// resolved and nowhere else.
type DotEnv struct {
	values map[string]string
	// origin records which file each name came from, so a human can be told
	// where a key they did not expect actually came from.
	origin map[string]string
}

// Lookup returns the value for name and where it came from.
func (d *DotEnv) Lookup(name string) (value, from string, ok bool) {
	if d == nil {
		return "", "", false
	}
	v, ok := d.values[name]
	if !ok {
		return "", "", false
	}
	return v, d.origin[name], true
}

// Names returns every name defined, for reporting. Order is unspecified.
func (d *DotEnv) Names() []string {
	if d == nil {
		return nil
	}
	out := make([]string, 0, len(d.values))
	for k := range d.values {
		out = append(out, k)
	}
	return out
}

// LoadDotEnv reads the user and project .env files.
//
// Project last, so a project file wins for a name both define: that is the
// convention every other tool with a .env follows, and a project file is
// normally something the person working in the repo wrote for it. Neither ever
// beats a real environment variable -- see Config.GetProviderKey. Missing
// files are the normal case and are not errors; unreadable ones are reported
// so a file that exists and did nothing is not silent.
func LoadDotEnv(workingDir string) (*DotEnv, []string) {
	d := &DotEnv{values: map[string]string{}, origin: map[string]string{}}
	var problems []string

	var paths []string
	if home, err := ResolveHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, DotEnvFile))
	}
	if strings.TrimSpace(workingDir) != "" {
		project := filepath.Join(workingDir, DotEnvFile)
		// Skip a project file that resolves to the user one -- running from
		// the packetcode home would otherwise read it twice and report every
		// key as project-scoped.
		if len(paths) == 0 || !samePath(project, paths[0]) {
			paths = append(paths, project)
		}
	}

	for _, path := range paths {
		vals, err := parseDotEnvFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				problems = append(problems, fmt.Sprintf("%s: %s", path, err))
			}
			continue
		}
		for k, v := range vals {
			d.values[k] = v
			d.origin[k] = path
		}
	}
	return d, problems
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// parseDotEnvFile reads one .env file.
//
// The dialect is the common one: KEY=VALUE per line, blank lines and lines
// starting with # ignored, an optional `export ` prefix accepted, and values
// optionally wrapped in single or double quotes. Escape sequences are NOT
// interpreted inside double quotes: an API key is an opaque token and quietly
// turning a backslash in one into something else would corrupt it.
func parseDotEnvFile(path string) (map[string]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	if info.Size() > maxDotEnvBytes {
		return nil, fmt.Errorf("file is %d bytes, over the %d byte cap", info.Size(), maxDotEnvBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxDotEnvBytes)
	for scanner.Scan() {
		name, value, ok := parseDotEnvLine(scanner.Text())
		if !ok {
			continue
		}
		out[name] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// parseDotEnvLine parses one line, returning ok=false for anything that is not
// an assignment (blank lines, comments, malformed lines).
func parseDotEnvLine(line string) (name, value string, ok bool) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	name, value, ok = strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", false
	}
	value = strings.TrimSpace(value)

	// Quotes are stripped only as a matched pair. An unmatched quote is part
	// of the value: keys do contain punctuation, and half-eating one would
	// produce a key that fails authentication for no visible reason.
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return name, value[1 : len(value)-1], true
		}
	}
	// An unquoted value may carry a trailing comment. A quoted one may not,
	// which is how a value containing " #" is written.
	if i := strings.Index(value, " #"); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}
	return name, value, true
}
