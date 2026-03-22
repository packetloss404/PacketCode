# Deferred Work

Outstanding items intentionally deferred to focus on shipping the core product. Full specs archived in `dev/archive/`.

---

## Remediation Phase 9: Code Signing & Distribution

**Status:** Not started | **Spec:** `dev/archive/remediation-plan/remediation/phase-09-code-signing-distribution.md`

- [ ] Windows code signing certificate (certificateThumbprint, digestAlgorithm, timestampUrl)
- [ ] macOS signing identity + entitlements
- [ ] Auto-update mechanism (tauri-plugin-updater)
- [ ] Signing key generation + CI secrets
- [ ] NSIS installer config (install path, start menu, images)
- [ ] DMG installer config (background, window size, app position)
- [ ] License agreement in installer
- [ ] Rollback strategy documentation

**Impact:** Without signing, Windows SmartScreen warns "Unknown Publisher" and macOS Gatekeeper blocks execution.

---

## Remediation Phase 10: Testing & Long-term Quality

**Status:** 5% complete | **Spec:** `dev/archive/remediation-plan/remediation/phase-10-testing-data-longterm.md`

### Frontend Tests
- [ ] Install vitest + @testing-library/react
- [ ] Add test script to package.json
- [ ] Create vitest.config.ts + test setup
- [ ] Write tests for stores (issueStore, flightStore, appStore)
- [ ] Write tests for critical components

### Rust Tests
- [ ] Unit tests for statusline.rs (especially iso_to_epoch)
- [ ] Unit tests for fs.rs, github.rs, pty.rs, memory.rs, spec.rs, insights.rs
- [ ] Integration tests for Tauri commands

### E2E Tests
- [ ] Playwright or WebDriver setup
- [ ] Frontend/backend contract verification

### Secure Storage
- [ ] GitHub token: migrate from plaintext file to OS keychain (keyring crate)
- [ ] Zeroize sensitive data in memory (zeroize crate)

### Data Quality
- [ ] Formal data versioning framework (CURRENT_VERSION per store, migrateData())
- [ ] Bundle size optimization (lazy loading views, light syntax highlighter build)

### Crash Reporting
- [ ] Rust panic hook for crash logging to disk
- [ ] Crash report viewer on next launch

---

## Vibe-Review Feature 2: Inline File Preview from Terminal

**Status:** Not started | **Spec:** `dev/archive/remediation-plan/vibe-review/feature-02-inline-file-preview-peek-from-terminal.md`

- [ ] Detect clickable file paths in terminal output
- [ ] Open inline preview panel on click
- [ ] Integration with FileExplorer or dedicated preview component

---

## Vibe-Review Feature 3: Session Persistence & Reconnection

**Status:** Not started | **Spec:** `dev/archive/remediation-plan/vibe-review/feature-03-session-persistence-reconnection.md`

- [ ] Save scrollback history to disk on session close
- [ ] Restore session state on app restart
- [ ] Reconnect to running PTY processes if still alive

---

## Vibe-Review Feature 5: Multi-Model A/B Comparison

**Status:** Not started | **Spec:** `dev/archive/remediation-plan/vibe-review/feature-05-multi-model-ab-comparison.md`

- [ ] "Dual fire" mode: send same prompt to Claude + Codex simultaneously
- [ ] Side-by-side comparison UI
- [ ] Shared prompt dispatch logic

---

## Vibe-Review Feature 10: Plugin / Extension System

**Status:** Partial (hardcoded modules only) | **Spec:** `dev/archive/remediation-plan/vibe-review/feature-10-plugin-extension-system.md`

- [ ] Plugin loading from user folder (not just built-in modules)
- [ ] Plugin manifest format for community contributions
- [ ] Plugin enable/disable from Settings
