package config

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The patcher's whole job is to change one value and nothing else, in files
// where a '=', a '#', a '[' or a newline may not mean what it looks like.
func TestPatchTOMLChangesOnlyTheNamedValue(t *testing.T) {
	cases := []struct {
		name string
		src  string
		edit tomlEdit
		want string
	}{
		{
			name: "the key name also appears in a comment and in a string value",
			src: "[default]\n" +
				"# model = \"do not touch this\"\n" +
				"provider = \"openai\"   # model = \"nor this\"\n" +
				"note = \"model = \\\"nor this either\\\"\"\n" +
				"model = \"gpt-4.1\"\n",
			edit: tomlEdit{path: []string{"default", "model"}, value: "gpt-5"},
			want: "[default]\n" +
				"# model = \"do not touch this\"\n" +
				"provider = \"openai\"   # model = \"nor this\"\n" +
				"note = \"model = \\\"nor this either\\\"\"\n" +
				"model = \"gpt-5\"\n",
		},
		{
			name: "a multi-line string holds a decoy assignment",
			src: "[behavior]\n" +
				"banner = \"\"\"\n" +
				"trust_mode = true\n" +
				"and a trailing quote \"\n" +
				"\"\"\"\n" +
				"trust_mode = false\n",
			edit: tomlEdit{path: []string{"behavior", "trust_mode"}, value: true},
			want: "[behavior]\n" +
				"banner = \"\"\"\n" +
				"trust_mode = true\n" +
				"and a trailing quote \"\n" +
				"\"\"\"\n" +
				"trust_mode = true\n",
		},
		{
			name: "a literal multi-line string holds a decoy table header",
			src: "[behavior]\n" +
				"banner = '''\n" +
				"[providers.openai]\n" +
				"'''\n" +
				"max_input_rows = 10\n",
			edit: tomlEdit{path: []string{"behavior", "max_input_rows"}, value: int64(20)},
			want: "[behavior]\n" +
				"banner = '''\n" +
				"[providers.openai]\n" +
				"'''\n" +
				"max_input_rows = 20\n",
		},
		{
			name: "an array spans lines and carries comments",
			src: "[mcp.git]\n" +
				"args = [\n" +
				"  \"mcp-server-git\", # command = \"decoy\"\n" +
				"  \"--repository\", \".\",\n" +
				"]\n" +
				"command = \"uvx\"\n",
			edit: tomlEdit{path: []string{"mcp", "git", "command"}, value: "uv"},
			want: "[mcp.git]\n" +
				"args = [\n" +
				"  \"mcp-server-git\", # command = \"decoy\"\n" +
				"  \"--repository\", \".\",\n" +
				"]\n" +
				"command = \"uv\"\n",
		},
		{
			name: "the same key name lives in two different tables",
			src:  "[providers.openai]\napi_key = \"first\"\n\n[providers.anthropic]\napi_key = \"second\"\n",
			edit: tomlEdit{path: []string{"providers", "anthropic", "api_key"}, value: "changed"},
			want: "[providers.openai]\napi_key = \"first\"\n\n[providers.anthropic]\napi_key = \"changed\"\n",
		},
		{
			name: "the file uses dotted keys instead of tables",
			src:  "providers.openai.api_key = \"old\"\nproviders.openai.default_model = \"gpt-4.1\"\n",
			edit: tomlEdit{path: []string{"providers", "openai", "api_key"}, value: "new"},
			want: "providers.openai.api_key = \"new\"\nproviders.openai.default_model = \"gpt-4.1\"\n",
		},
		{
			name: "a quoted table name contains a dot",
			src:  "[providers.\"my.provider\"]\napi_key = \"old\"\n",
			edit: tomlEdit{path: []string{"providers", "my.provider", "api_key"}, value: "new"},
			want: "[providers.\"my.provider\"]\napi_key = \"new\"\n",
		},
		{
			name: "a quoted key sits beside its bare namesake",
			src:  "[permissions.tools]\n\"execute command\" = \"ask\"\nexecute_command = \"allow\"\n",
			edit: tomlEdit{path: []string{"permissions", "tools", "execute command"}, value: "deny"},
			want: "[permissions.tools]\n\"execute command\" = \"deny\"\nexecute_command = \"allow\"\n",
		},
		{
			name: "an inline table sits on the line above",
			src:  "[permissions]\nprofiles = { ci = \"ask\" }\nprofile = \"balanced\"\n",
			edit: tomlEdit{path: []string{"permissions", "profile"}, value: "read_only"},
			want: "[permissions]\nprofiles = { ci = \"ask\" }\nprofile = \"read_only\"\n",
		},
		{
			name: "the last line has no newline",
			src:  "[default]\nprovider = \"openai\"",
			edit: tomlEdit{path: []string{"default", "provider"}, value: "anthropic"},
			want: "[default]\nprovider = \"anthropic\"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := patchTOML(tc.src, []tomlEdit{tc.edit})
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			var parsed map[string]any
			_, err = toml.Decode(got, &parsed)
			require.NoError(t, err, "the patched document must still parse")
		})
	}
}

func TestPatchTOMLInsertsWhereTheKeyBelongs(t *testing.T) {
	cases := []struct {
		name string
		src  string
		edit tomlEdit
		want string
	}{
		{
			name: "into the table that already exists",
			src:  "[providers.openai]\napi_key = \"k\"\n\n[behavior]\ntrust_mode = false\n",
			edit: tomlEdit{path: []string{"providers", "openai", "default_model"}, value: "gpt-5"},
			want: "[providers.openai]\napi_key = \"k\"\ndefault_model = \"gpt-5\"\n\n[behavior]\ntrust_mode = false\n",
		},
		{
			name: "as a dotted key under the deepest existing table",
			src:  "[providers]\n\n[behavior]\ntrust_mode = false\n",
			edit: tomlEdit{path: []string{"providers", "openai", "api_key"}, value: "k"},
			want: "[providers]\nopenai.api_key = \"k\"\n\n[behavior]\ntrust_mode = false\n",
		},
		{
			name: "in a new table appended at the end",
			src:  "[behavior]\ntrust_mode = false\n",
			edit: tomlEdit{path: []string{"providers", "openai", "api_key"}, value: "k"},
			want: "[behavior]\ntrust_mode = false\n\n[providers.openai]\napi_key = \"k\"\n",
		},
		{
			name: "above the first table header, because a root key written below one joins it",
			src:  "# my config\n[behavior]\ntrust_mode = false\n",
			edit: tomlEdit{path: []string{"schema_version"}, value: int64(2)},
			want: "schema_version = 2\n# my config\n[behavior]\ntrust_mode = false\n",
		},
		{
			name: "before the comment that introduces the next table",
			src:  "[behavior]\ntrust_mode = false\n\n# providers live here\n[providers.openai]\napi_key = \"k\"\n",
			edit: tomlEdit{path: []string{"behavior", "max_input_rows"}, value: int64(10)},
			want: "[behavior]\ntrust_mode = false\nmax_input_rows = 10\n\n# providers live here\n[providers.openai]\napi_key = \"k\"\n",
		},
		{
			name: "onto a file whose last line has no newline",
			src:  "[behavior]\ntrust_mode = false",
			edit: tomlEdit{path: []string{"behavior", "max_input_rows"}, value: int64(10)},
			want: "[behavior]\ntrust_mode = false\nmax_input_rows = 10\n",
		},
		{
			name: "into an empty document",
			src:  "# nothing here yet\n",
			edit: tomlEdit{path: []string{"providers", "openai", "api_key"}, value: "k"},
			want: "# nothing here yet\n\n[providers.openai]\napi_key = \"k\"\n",
		},
		{
			name: "quoting a table name that is not a bare key",
			src:  "[behavior]\ntrust_mode = false\n",
			edit: tomlEdit{path: []string{"providers", "my provider", "api_key"}, value: "k"},
			want: "[behavior]\ntrust_mode = false\n\n[providers.\"my provider\"]\napi_key = \"k\"\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := patchTOML(tc.src, []tomlEdit{tc.edit})
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			var parsed map[string]any
			_, err = toml.Decode(got, &parsed)
			require.NoError(t, err, "the patched document must still parse")
		})
	}
}

// A CRLF file that gains a lone LF line stops being a file the user's editor
// and their diff tool agree about. This is the platform packetcode is built
// on, so it is not an edge case.
func TestPatchTOMLKeepsCRLFLineEndings(t *testing.T) {
	src := "[providers.openai]\r\napi_key = \"old\"\r\n"
	got, err := patchTOML(src, []tomlEdit{
		{path: []string{"providers", "openai", "api_key"}, value: "new"},
		{path: []string{"providers", "openai", "default_model"}, value: "gpt-5"},
		{path: []string{"behavior", "trust_mode"}, value: true},
	})
	require.NoError(t, err)
	assert.Equal(t,
		"[providers.openai]\r\napi_key = \"new\"\r\ndefault_model = \"gpt-5\"\r\n\r\n[behavior]\r\ntrust_mode = true\r\n",
		got)
	assert.NotRegexp(t, "[^\r]\n", got, "a lone LF crept into a CRLF file")
}

func TestPatchTOMLRemovesAClearedKey(t *testing.T) {
	src := "[providers.openai]\napi_key = \"k\"\nreasoning_effort = \"high\"\ndefault_model = \"gpt-5\"\n"
	got, err := patchTOML(src, []tomlEdit{{path: []string{"providers", "openai", "reasoning_effort"}, remove: true}})
	require.NoError(t, err)
	assert.Equal(t, "[providers.openai]\napi_key = \"k\"\ndefault_model = \"gpt-5\"\n", got)

	// Clearing a key that is not there is what an already-absent optional
	// setting looks like, and must not be an error.
	unchanged, err := patchTOML(got, []tomlEdit{{path: []string{"providers", "openai", "reasoning_effort"}, remove: true}})
	require.NoError(t, err)
	assert.Equal(t, got, unchanged)
}

// Refusing by name beats writing an approximation: the file the user was
// trying to protect is the one at stake.
func TestPatchTOMLRefusesWhatItCannotExpress(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		edit    tomlEdit
		wantErr string
	}{
		{
			name:    "a key inside an array of tables has one value per element",
			src:     "[[permissions.rules]]\ntool = \"spawn_agent\"\naction = \"deny\"\n",
			edit:    tomlEdit{path: []string{"permissions", "rules", "action"}, value: "allow"},
			wantErr: "array of tables",
		},
		{
			name:    "replacing the array of tables itself",
			src:     "[[permissions.rules]]\ntool = \"spawn_agent\"\naction = \"deny\"\n",
			edit:    tomlEdit{path: []string{"permissions", "rules"}, value: []any{}},
			wantErr: "array of tables",
		},
		{
			name:    "the path names a table declared by a header",
			src:     "[providers.openai]\napi_key = \"k\"\n",
			edit:    tomlEdit{path: []string{"providers", "openai"}, value: "k"},
			wantErr: "is a table, not a setting",
		},
		{
			name:    "the path names a table built from dotted keys",
			src:     "providers.openai.api_key = \"k\"\n",
			edit:    tomlEdit{path: []string{"providers", "openai"}, value: "k"},
			wantErr: "is a table, not a setting",
		},
		{
			name:    "a value type TOML has no literal for",
			src:     "[behavior]\ntrust_mode = false\n",
			edit:    tomlEdit{path: []string{"behavior", "trust_mode"}, value: make(chan int)},
			wantErr: "cannot be written as TOML",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := patchTOML(tc.src, []tomlEdit{tc.edit})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestRenderTOMLValueRoundTrips(t *testing.T) {
	values := map[string]any{
		"text":      "a \"quoted\" \\ path\nwith a newline\tand a tab",
		"unicode":   "café ✓",
		"control":   "bell\x07",
		"truth":     true,
		"count":     int64(42),
		"rate":      1.0,
		"tiny":      0.25,
		"list":      []any{"a", "b"},
		"mixed":     []any{int64(1), 2.5, false},
		"tables":    []map[string]any{{"tool": "x", "action": "deny"}},
		"inline":    map[string]any{"a.b": "dotted key", "plain": int64(1)},
		"emptylist": []any{},
	}
	var b strings.Builder
	for key, value := range values {
		text, err := renderTOMLValue(value)
		require.NoError(t, err, key)
		b.WriteString(renderTOMLKeyPart(key) + " = " + text + "\n")
	}
	var parsed map[string]any
	_, err := toml.Decode(b.String(), &parsed)
	require.NoError(t, err, b.String())

	assert.Equal(t, values["text"], parsed["text"])
	assert.Equal(t, values["unicode"], parsed["unicode"])
	assert.Equal(t, values["control"], parsed["control"])
	assert.Equal(t, true, parsed["truth"])
	assert.Equal(t, int64(42), parsed["count"])
	// 1.0 must not come back as an integer: that is a different TOML type and
	// a different Go field.
	assert.Equal(t, 1.0, parsed["rate"])
	assert.Equal(t, []any{"a", "b"}, parsed["list"])
	assert.Equal(t, []any{map[string]any{"tool": "x", "action": "deny"}}, parsed["tables"])
	assert.Equal(t, map[string]any{"a.b": "dotted key", "plain": int64(1)}, parsed["inline"])
}

func TestScanTOMLDocReportsMalformedSource(t *testing.T) {
	for _, src := range []string{
		"[default\nprovider = \"openai\"\n",
		"[default]\nprovider = \"unterminated\n",
		"[default]\nprovider = \"\"\"unterminated\n",
		"[default]\nargs = [\"a\", \"b\"\n",
		"[default]\nprovider\n",
		"[default]\nprovider =\n",
	} {
		_, err := scanTOMLDoc(src)
		assert.Error(t, err, "expected %q to be refused", src)
	}
}
