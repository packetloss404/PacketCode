# Phase 5: CI/CD Pipeline Hardening

**Priority:** P1 — This Sprint
**Timeline:** Week 2
**Effort:** Medium (1–2 days total)
**Risk Level:** High (process gap)
**Owners:** DevOps, Backend Lead

---

## Overview

The current CI pipeline is minimal: it runs frontend linting/build and Rust unit tests, both on Ubuntu only. It never builds the actual Tauri desktop application, never scans for dependency vulnerabilities, never tests on Windows or macOS, and has no quality gates beyond "does it compile." This means integration failures, platform-specific bugs, and known CVEs in the supply chain go undetected until manual testing or production incidents.

This phase transforms the pipeline into a production-grade CI/CD system.

---

## Finding F-06: No Dependency Vulnerability Scanning in CI

### Severity: High

### Description

The CI pipeline defined in `.github/workflows/ci.yml` runs `pnpm install --frozen-lockfile`, `pnpm lint`, `pnpm build`, and `cargo test`, but never runs any dependency vulnerability scanning. Known CVEs in transitive dependencies — both Rust crates and npm packages — go undetected indefinitely.

### Evidence

```yaml
# .github/workflows/ci.yml, lines 9-41
jobs:
  frontend:
    name: Frontend lint and build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with:
          version: 9
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: pnpm
      - run: pnpm install --frozen-lockfile
      - name: ESLint
        run: pnpm lint
      - name: Build
        run: pnpm build
      # No pnpm audit step

  backend:
    name: Rust tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: dtolnay/rust-toolchain@stable
      - name: Install system dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y libwebkit2gtk-4.1-dev libappindicator3-dev librsvg2-dev patchelf libgtk-3-dev
      - name: Cargo test
        working-directory: src-tauri
        run: cargo test
      # No cargo audit step
```

### Impact

- Supply chain attacks are the #1 threat vector for desktop applications. The `event-stream` incident (npm) and `crates.io` typosquatting attacks demonstrate the risk.
- Known vulnerabilities in transitive dependencies (e.g., in `reqwest`'s dependency tree, or in `react-syntax-highlighter`'s dependency tree) would go undetected.
- Any compliance or security review would flag the absence of dependency scanning as a critical gap.

### Remediation

Add vulnerability scanning steps to both CI jobs:

**Frontend job — add `pnpm audit`:**

```yaml
  frontend:
    name: Frontend lint and build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with:
          version: 9
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: pnpm
      - run: pnpm install --frozen-lockfile
      - name: Audit dependencies
        run: pnpm audit --audit-level=high
      - name: ESLint
        run: pnpm lint
      - name: Build
        run: pnpm build
```

**Backend job — add `cargo audit`:**

```yaml
  backend:
    name: Rust tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: dtolnay/rust-toolchain@stable
      - name: Install system dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y libwebkit2gtk-4.1-dev libappindicator3-dev librsvg2-dev patchelf libgtk-3-dev
      - name: Install cargo-audit
        run: cargo install cargo-audit
      - name: Audit Rust dependencies
        working-directory: src-tauri
        run: cargo audit
      - name: Cargo test
        working-directory: src-tauri
        run: cargo test
```

**Alternative:** Use the `rustsec/audit-check` GitHub Action for faster execution (no cargo-audit compile step):

```yaml
      - uses: rustsec/audit-check@v2
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
```

### Complementary: Automated Dependency Updates

Configure Dependabot or Renovate to create automated PRs for dependency updates:

```yaml
# .github/dependabot.yml
version: 2
updates:
  - package-ecosystem: "npm"
    directory: "/"
    schedule:
      interval: "weekly"
    open-pull-requests-limit: 5

  - package-ecosystem: "cargo"
    directory: "/src-tauri"
    schedule:
      interval: "weekly"
    open-pull-requests-limit: 5
```

### Risk of Change

None. These are additive CI steps. If a vulnerability is found, the audit step will fail the build — which is the desired behavior.

---

## Finding F-07: CI Never Builds the Tauri Application

### Severity: High

### Description

The CI pipeline builds the frontend (Vite) and runs Rust unit tests independently, but never runs `pnpm tauri build` to build the actual desktop application. This means integration issues between the frontend and backend — such as missing command registrations, IPC type mismatches, packaging errors, or Tauri plugin configuration problems — are never caught by CI.

### Evidence

The two jobs are completely independent:
- Frontend job: `pnpm build` (Vite build only — produces web assets)
- Backend job: `cargo test` (Rust tests only — no Tauri context)

Neither job ever runs the Tauri CLI, which is the only tool that combines both layers into a desktop application.

### Impact

- A missing `#[tauri::command]` attribute on a new function would not be caught
- A Tauri capability misconfiguration would not be caught
- Plugin registration errors would not be caught
- Icon/resource issues would not be caught
- The build could be broken on `main` without anyone knowing until someone manually runs `pnpm tauri build`

### Remediation

Add a Tauri build job that compiles the full desktop application:

```yaml
  tauri-build:
    name: Tauri build
    needs: [frontend, backend]
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4

      - uses: pnpm/action-setup@v4
        with:
          version: 9
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: pnpm
      - uses: dtolnay/rust-toolchain@stable

      - name: Install Linux dependencies
        if: runner.os == 'Linux'
        run: |
          sudo apt-get update
          sudo apt-get install -y libwebkit2gtk-4.1-dev libappindicator3-dev librsvg2-dev patchelf libgtk-3-dev

      - run: pnpm install --frozen-lockfile

      - name: Build Tauri app
        run: pnpm tauri build
        env:
          TAURI_SIGNING_PRIVATE_KEY: ""  # Skip signing in CI for now
```

**Cost-conscious alternative:** If running on all three platforms is too expensive for every commit, run the full matrix only on `main` pushes and PRs, and use a single platform for branch pushes:

```yaml
  tauri-build:
    name: Tauri build
    needs: [frontend, backend]
    runs-on: ubuntu-latest  # Single platform for basic integration check
    steps:
      # ... same as above but Ubuntu only
```

### Risk of Change

None. This is an additive CI job. It may initially reveal existing build issues that were previously hidden.

---

## Finding F-19: CI Only Runs on Ubuntu

### Severity: Medium

### Description

Both CI jobs use `runs-on: ubuntu-latest`. PacketCode primarily targets Windows (the CLAUDE.md mentions Windows-specific environment setup and the default `projectPath` is a Windows path) and presumably macOS. Platform-specific bugs are never tested:

- **Windows:** `.cmd` wrapper resolution for CLI tools, path separator handling, PTY behavior differences
- **macOS:** Different webview engine (WKWebView vs WebKitGTK), signing requirements, sandbox behavior
- **Linux:** Likely the least-tested target despite being the CI platform

### Evidence

```yaml
# .github/workflows/ci.yml
  frontend:
    runs-on: ubuntu-latest  # <-- Only Ubuntu

  backend:
    runs-on: ubuntu-latest  # <-- Only Ubuntu
```

### Impact

- Windows-specific PTY issues are never caught
- Path handling differences between operating systems are never tested
- macOS-specific webview behavior is never validated
- Users on non-Linux platforms may encounter platform-specific bugs that CI never catches

### Remediation

Add an OS matrix to the backend job (frontend is typically platform-independent):

```yaml
  backend:
    name: Rust tests
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: dtolnay/rust-toolchain@stable

      - name: Install Linux dependencies
        if: runner.os == 'Linux'
        run: |
          sudo apt-get update
          sudo apt-get install -y libwebkit2gtk-4.1-dev libappindicator3-dev librsvg2-dev patchelf libgtk-3-dev

      - name: Cargo test
        working-directory: src-tauri
        run: cargo test
```

### Risk of Change

None. May reveal platform-specific test failures that should be fixed.

---

## Finding F-19b: No Test Coverage Reporting or Quality Gates

### Severity: Medium

### Description

The CI pipeline has no coverage reporting, no SAST (Static Application Security Testing) tools, no license scanning, and no quality gates beyond "does it compile and pass linting."

### Evidence

No coverage tools (`cargo-tarpaulin`, `cargo-llvm-cov`, `vitest --coverage`) are present. No SAST tools (`CodeQL`, `Semgrep`) are configured. No license compliance checking is performed.

### Impact

- No visibility into test coverage trends
- No automated detection of common vulnerability patterns (SQL injection, XSS, etc.)
- No automated license compliance — a risk for commercial distribution
- No quality gates mean regressions can merge without detection

### Remediation

Add quality tools progressively:

**Phase 5a — Coverage (this sprint):**

```yaml
      - name: Install tarpaulin
        run: cargo install cargo-tarpaulin
      - name: Run coverage
        working-directory: src-tauri
        run: cargo tarpaulin --out xml
      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          file: src-tauri/cobertura.xml
```

**Phase 5b — SAST (next sprint):**

```yaml
  codeql:
    name: CodeQL Analysis
    runs-on: ubuntu-latest
    permissions:
      security-events: write
    steps:
      - uses: actions/checkout@v4
      - uses: github/codeql-action/init@v3
        with:
          languages: javascript, typescript
      - uses: github/codeql-action/analyze@v3
```

**Phase 5c — License scanning (later):**

```yaml
      - name: License check
        run: npx license-checker --failOn "GPL-3.0;AGPL-3.0"
```

### Risk of Change

None. All additive.

---

## Finding F-19c: Branch Protection Unknown

### Severity: Low

### Description

The CI triggers on `push: branches: [main]` and `pull_request`, which is correct. However, whether GitHub branch protection rules require CI checks to pass before merging is a repository-level setting not visible in the codebase.

### Remediation

Verify and enable branch protection on `main`:
- Require status checks to pass before merging (select `frontend` and `backend` jobs)
- Require branches to be up-to-date before merging
- Consider requiring pull request reviews

### Risk of Change

None.

---

## Recommended Final CI Configuration

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
  workflow_dispatch:

jobs:
  frontend:
    name: Frontend lint and build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with:
          version: 9
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: pnpm
      - run: pnpm install --frozen-lockfile
      - name: Audit npm dependencies
        run: pnpm audit --audit-level=high
      - name: ESLint
        run: pnpm lint
      - name: Build
        run: pnpm build

  backend:
    name: Rust tests
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: dtolnay/rust-toolchain@stable
      - name: Install Linux dependencies
        if: runner.os == 'Linux'
        run: |
          sudo apt-get update
          sudo apt-get install -y libwebkit2gtk-4.1-dev libappindicator3-dev librsvg2-dev patchelf libgtk-3-dev
      - name: Audit Rust dependencies
        if: runner.os == 'Linux'
        run: cargo install cargo-audit && cargo audit
        working-directory: src-tauri
      - name: Cargo test
        working-directory: src-tauri
        run: cargo test

  tauri-build:
    name: Tauri integration build
    needs: [frontend, backend]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with:
          version: 9
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: pnpm
      - uses: dtolnay/rust-toolchain@stable
      - name: Install Linux dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y libwebkit2gtk-4.1-dev libappindicator3-dev librsvg2-dev patchelf libgtk-3-dev
      - run: pnpm install --frozen-lockfile
      - name: Build Tauri app
        run: pnpm tauri build
```

---

## Testing Checklist

- [ ] Add `pnpm audit --audit-level=high` to frontend CI job
- [ ] Add `cargo audit` to backend CI job
- [ ] Add OS matrix to backend job (at least Ubuntu + Windows)
- [ ] Add Tauri build integration job
- [ ] Run updated pipeline and verify all jobs pass
- [ ] Fix any dependency vulnerabilities surfaced by audit steps
- [ ] Configure branch protection rules on `main`
- [ ] Set up Dependabot or Renovate for automated dependency PRs

---

## References

- `.github/workflows/ci.yml` — lines 1–41
- `src-tauri/Cargo.toml` — dependency declarations
- `package.json` — dependency declarations
- Tauri CI documentation: GitHub Actions setup
- RustSec Advisory Database: cargo-audit
