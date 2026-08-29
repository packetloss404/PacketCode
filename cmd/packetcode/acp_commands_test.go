package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateCommandHome points os.UserHomeDir at a temp dir on both POSIX and
// Windows so config.UserCommandsDir resolves inside the test sandbox, and
// clears PACKETCODE_HOME so a developer's own override cannot leak in.
func isolateCommandHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PACKETCODE_HOME", "")
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	return dir
}

func writeCommandFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

func TestPacketCommandCatalogReportsOnlyMarkdownCommands(t *testing.T) {
	home := isolateCommandHome(t)
	project := t.TempDir()

	writeCommandFile(t, filepath.Join(home, ".packetcode", "commands"), "audit.md",
		"---\ndescription: Security-review the working tree\n---\nAudit every changed file.\n")
	writeCommandFile(t, filepath.Join(project, ".packetcode", "commands"), "deploy.md",
		"---\ndescription: Ship to an environment\n---\nDeploy to $ARGUMENTS and report back.\n")

	catalog := &packetCommandCatalog{}
	commands, err := catalog.ListCommands(project)
	require.NoError(t, err)
	require.Len(t, commands, 2, "expected exactly the two markdown commands, got %+v", commands)

	byName := map[string]int{}
	for i, cmd := range commands {
		byName[cmd.Name] = i
		// Not one built-in may appear: every entry in app.SlashCommands is a
		// TUI affordance the ACP surface cannot honour.
		assert.NotEqual(t, "builtin", cmd.Source)
		assert.NotEqual(t, "built-in", cmd.Source)
	}
	require.Contains(t, byName, "audit")
	require.Contains(t, byName, "deploy")

	audit := commands[byName["audit"]]
	assert.Equal(t, "user", audit.Source)
	assert.Equal(t, "Security-review the working tree", audit.Description)
	// Every custom command takes arguments: the placeholder places them, and
	// without one they are appended. A hint of "" would tell a client this
	// command accepts none, which stopped being true when Expand stopped
	// dropping them.
	assert.Equal(t, "[arguments]", audit.ArgumentHint)
	assert.Contains(t, audit.Body, "Audit every changed file.")

	deploy := commands[byName["deploy"]]
	assert.Equal(t, "project", deploy.Source)
	assert.Equal(t, "Ship to an environment", deploy.Description)
	assert.Equal(t, "[arguments]", deploy.ArgumentHint)
	assert.Contains(t, deploy.Body, "$ARGUMENTS")
}

func TestPacketCommandCatalogWithoutCwdSkipsProjectCommands(t *testing.T) {
	home := isolateCommandHome(t)
	project := t.TempDir()

	writeCommandFile(t, filepath.Join(home, ".packetcode", "commands"), "audit.md", "Audit the tree.\n")
	writeCommandFile(t, filepath.Join(project, ".packetcode", "commands"), "deploy.md", "Deploy it.\n")

	commands, err := (&packetCommandCatalog{}).ListCommands("")
	require.NoError(t, err)
	require.Len(t, commands, 1)
	assert.Equal(t, "audit", commands[0].Name)
	assert.Equal(t, "user", commands[0].Source)
	// A file with no front matter still gets a usable description.
	assert.NotEmpty(t, commands[0].Description)
}

func TestPacketCommandCatalogProjectOverridesUser(t *testing.T) {
	home := isolateCommandHome(t)
	project := t.TempDir()

	writeCommandFile(t, filepath.Join(home, ".packetcode", "commands"), "audit.md",
		"---\ndescription: user copy\n---\nUser body.\n")
	writeCommandFile(t, filepath.Join(project, ".packetcode", "commands"), "audit.md",
		"---\ndescription: project copy\n---\nProject body.\n")

	commands, err := (&packetCommandCatalog{}).ListCommands(project)
	require.NoError(t, err)
	require.Len(t, commands, 1)
	assert.Equal(t, "project", commands[0].Source)
	assert.Equal(t, "project copy", commands[0].Description)
}

func TestPacketCommandCatalogIsEmptyWithoutCommandFiles(t *testing.T) {
	isolateCommandHome(t)

	commands, err := (&packetCommandCatalog{}).ListCommands(t.TempDir())
	require.NoError(t, err)
	// Built-ins are deliberately withheld, so a bare install offers nothing.
	assert.Empty(t, commands)
}

func TestPacketProjectFileIndexFindsFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "components"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "App.tsx"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "components", "Composer.tsx"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o600))
	// Binaries and skipped trees must never be offered as mentions.
	require.NoError(t, os.WriteFile(filepath.Join(root, "logo.png"), []byte("x"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "node_modules", "dep"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "node_modules", "dep", "index.js"), []byte("x"), 0o600))

	index := &packetProjectFileIndex{}

	all, err := index.SearchFiles(root, "", 50)
	require.NoError(t, err)
	assert.Contains(t, all, "src/App.tsx")
	assert.Contains(t, all, "README.md")
	assert.NotContains(t, all, "logo.png")
	assert.NotContains(t, all, "node_modules/dep/index.js")

	// A basename prefix ranks as a match even mid-path.
	hits, err := index.SearchFiles(root, "compo", 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"src/components/Composer.tsx"}, hits)

	// Queries are case-insensitive.
	hits, err = index.SearchFiles(root, "READ", 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"README.md"}, hits)

	// No match is an empty slice, never an error.
	hits, err = index.SearchFiles(root, "zzzznope", 20)
	require.NoError(t, err)
	assert.Empty(t, hits)
}

func TestPacketProjectFileIndexRejectsNonDirectories(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "notadir.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	index := &packetProjectFileIndex{}

	_, err := index.SearchFiles(file, "", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")

	// A missing directory surfaces the stat error to the ACP layer (-32603).
	_, err = index.SearchFiles(filepath.Join(root, "missing"), "", 10)
	require.Error(t, err)
}

func TestRankProjectFilesOrdersPrefixMatchesFirst(t *testing.T) {
	paths := []string{
		"docs/composer-notes.md",
		"src/zeta/composer.ts",
		"composer/main.go",
		"src/components/Composer.tsx",
		// Substring-only: neither the path nor the basename starts with the
		// needle, so this ranks below every prefix hit despite sorting first
		// lexically.
		"app/composer-helpers/util.go",
		"unrelated.txt",
	}

	// Tier 1 is a prefix on the full path or on the basename; tier 2 is a
	// substring anywhere. Each tier sorts lexically, tier 1 first.
	got := rankProjectFiles(paths, "composer", 10)
	assert.Equal(t, []string{
		"composer/main.go",
		"docs/composer-notes.md",
		"src/components/Composer.tsx",
		"src/zeta/composer.ts",
		"app/composer-helpers/util.go",
	}, got)

	// The limit truncates after ranking, so the best matches survive.
	assert.Equal(t, []string{"composer/main.go", "docs/composer-notes.md"},
		rankProjectFiles(paths, "composer", 2))

	// An empty query returns the head of the list in its original order.
	assert.Equal(t, paths[:2], rankProjectFiles(paths, "", 2))

	// Degenerate limits and no-match queries yield an empty slice, not nil.
	assert.Equal(t, []string{}, rankProjectFiles(paths, "composer", 0))
	assert.Equal(t, []string{}, rankProjectFiles(paths, "nothinghere", 10))
	assert.Equal(t, []string{}, rankProjectFiles(nil, "x", 10))
}
