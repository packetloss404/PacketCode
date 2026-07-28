# Command and File Autocomplete

The autocomplete component serves two input modes above the prompt.

## Slash Commands

Typing `/` at the start of the input opens built-in and markdown-backed command suggestions. Prefix matches rank before normalized substring matches.

| Key | Action |
| --- | --- |
| Up/Down, Ctrl+N/P, Ctrl+J/K | Move. |
| Tab | Accept the highlighted command. |
| Enter | Accept a bare command; otherwise submit normally. |
| Esc | Close without deleting input. |

Accepting `/provider` or `/model` opens its picker. A space after the command closes completion. Unknown slash commands produce a local error; `//text` escapes a literal slash prompt.

## File Mentions

Typing `@` as the first character after whitespace opens fuzzy project-file suggestions. The index prefers:

```text
git ls-files --cached --others --exclude-standard
```

and falls back to a bounded directory walk that skips VCS, dependency, build, virtual-environment, hidden, and obvious binary paths. Results are project-relative and use the shared normalized matcher, with path/basename prefix matches promoted.

Tab or Enter replaces only the active token with `@<path> `, preserving earlier prompt text. On submission, root-scoped text files are expanded into bounded model context while the visible user message retains the literal mention.

Mention completion follows the caret within multiline input. Accepting a match replaces only the active `@` token, preserves surrounding text and whitespace, and restores the caret after the inserted path.

Implementation: `internal/ui/components/autocomplete`, `internal/app/mention_token.go`, and `internal/app/fileindex.go`.
