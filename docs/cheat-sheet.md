# packetcode — Terminal Cheat Sheet

## Start from your shell

```text
packetcode                                      # configured default
packetcode --provider codex                     # Codex/ChatGPT login
packetcode --provider codex --model gpt-5.6-sol
packetcode --provider ollama                    # local Ollama, no key
packetcode --resume <session-id>
packetcode --permission-mode auto               # also: ask, accept-edits, read-only, bypass
packetcode --trust                              # bypass (deny rules still apply)
packetcode doctor --check providers             # also: permissions, mcp
packetcode doctor --json
```

## Type these in packetcode

| Intent | Command / example |
| --- | --- |
| Provider and model | `/provider`, `/provider codex`, `/provider add <slug>`, `/model`, `/model gpt-5.6-sol` |
| Codex reasoning | `/effort`, `/effort high`, `/effort max`, `/effort default` |
| Permissions | `/permissions`, `/permissions profiles`, `/permissions profile auto`, `/permissions reset`, `/plan on`, `/trust on` |
| Include a file | `Review @internal/app/app.go and fix the focus bug` (`@` opens completion) |
| Read-only agent | `/spawn audit the current diff` |
| Chosen agent model | `/spawn --provider codex --model gpt-5.6-sol audit auth` |
| Isolated write agent | `/spawn --write fix the focused tests` |
| Agent control | `/agents`, `/agents <id>`, `/jobs [id]`, `/jobs resubmit [id]`, `/cancel <id|all>` |
| Computers | `/computers`, `/computers status <name>` (registry-only) |
| Workflow | `/workflows validate <name>`, `/workflows run review target="the staged diff"`, `/workflows list`, `/workflows stop all` |
| Repeat work | `/loop Continue until complete`, `/loop 10m /workflows run review`, `/loop list`, `/loop stop all` |
| Pending prompts | `/queue`, `/queue drop <n>`, `/queue clear` |
| Sessions/context | `/sessions`, `/sessions resume <id>`, `/sessions rename <name>`, `/compact --keep 10` |
| Usage/recovery | `/cost`, `/cost reset --yes`, `/undo`, `/transcript` |
| Ollama | `/ollama status`, `/ollama models`, `/ollama ps`, `/ollama pull <model>` |
| MCP | `/mcp`, `/mcp status <name>`, `/mcp tools <name>`, `/mcp logs <name>`, `/mcp restart <name>` (stdio only) |
| Literal slash | `//not-a-command` sends `/not-a-command` as the prompt |

## Keys worth remembering

`Enter` send · `Alt+Enter` / `\` then `Enter` newline · `Ctrl+J` newline except when completion is open (moves down) · `Shift+Tab` cycle Manual → Accept Edits → Auto → Plan · `Ctrl+P` provider · `/model` model picker (`Ctrl+M` when distinctly reported) · `Left` on an empty prompt agents · `Ctrl+C` cancel/clear draft/quit when empty · `Ctrl+D` quit when empty · `Ctrl+L` or `/clear` clear visible output, keep session · `/help` everything else

Write agents work in separate git worktrees; their changes are not merged automatically. Completed agent results stay out of foreground context until you inject/collect them.
