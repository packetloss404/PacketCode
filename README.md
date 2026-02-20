# PacketCode

A custom desktop IDE for running multiple Claude Code sessions side by side, built with Tauri 2 + React 19 + TypeScript.

## Features

- **Multi-session terminals** — Run as many Claude Code sessions as you want in a flexible grid layout
- **Real terminal emulation** — Full xterm.js terminals with PTY backend (not parsed output)
- **Session tab bar** — Track all sessions with live status labels ("Crunching...", "Brewing...", "Cogitated for 2m 15s")
- **Issue tracker** — Built-in kanban board with drag-and-drop, filters, priorities, labels, and epics
- **View switching** — Claude, Issues, History, and Tools views (Ctrl+Shift+1-4)
- **Flexible grid** — Sessions auto-arrange; closing one makes others expand to fill the space
- **Custom dark theme** — Terminal-style UI with green accent colors
- **Frameless window** — Custom titlebar with native window controls

## Tech Stack

- **Frontend**: React 19, TypeScript, Vite 6, Tailwind CSS, Zustand 5
- **Backend**: Tauri 2 (Rust), portable-pty for terminal management
- **Terminal**: @xterm/xterm with fit and web-links addons

## Prerequisites

- [Node.js](https://nodejs.org/) (v18+)
- [Rust](https://rustup.rs/)
- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) installed and on PATH

## Development

```bash
npm install
npm run tauri dev
```

## Build

```bash
npm run tauri build
```

## Keyboard Shortcuts

| Shortcut | Action |
|---|---|
| Ctrl+\\ | New session pane |
| Ctrl+1-4 | Switch between panes |
| Ctrl+Shift+1 | Claude view |
| Ctrl+Shift+2 | Issues view |
| Ctrl+Shift+3 | History view |
| Ctrl+Shift+4 | Tools view |

## License

MIT
