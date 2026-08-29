package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A pending skill must not be printed with a leading slash. The slash is the
// listing's claim that you can type the name, and for a skill that is not
// loaded the program will not run it -- a promise it does not keep is worse
// than no marker at all.
func TestSkillsList_PendingSkillIsNotShownAsTypeable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PACKETCODE_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	ws := t.TempDir()
	dir := filepath.Join(ws, ".claude", "skills", "deploy")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte("---\ndescription: Repo supplied\n---\nbody\n"), 0o600))

	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(ws))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, runSkillsList(nil, &stdout, &stderr))

	out := stdout.String()
	require.Contains(t, out, "NOT LOADED", "a pending skill must say so")
	require.Contains(t, out, "/skills allow deploy", "and say how to enable it")
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "deploy") && strings.HasPrefix(strings.TrimSpace(line), "/deploy") {
			t.Fatalf("a pending skill was printed as typeable:\n%s", line)
		}
	}
}
