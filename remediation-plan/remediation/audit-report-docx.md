PACKETCODE CODEBASE AUDIT REPORT

Date: 2026-02-24
Version: 0.1.0
Auditor: Static analysis, read-only, no modifications
Framework: Tauri v2 (Rust) + React 19 + TypeScript + Vite 6


SECTION 0: EXECUTIVE SUMMARY

Overall Health Scores (scale 0 to 10)

Security: 3 out of 10
Unrestricted process spawning from webview, no command allowlist in PTY creation, arbitrary filesystem path traversal on multiple endpoints.

Stability: 6 out of 10
Good error propagation in most Rust code. Blocking I/O on async runtime and one panic-risk path remain.

Maintainability: 7 out of 10
Clean Zustand patterns, TypeScript strict mode, consistent conventions. Some IPC calls bypass the centralized wrapper module.

Desktop Safety: 3 out of 10
No code signing, no auto-updater. Unsigned installers will be blocked by OS gatekeepers on Windows and macOS.

Performance: 6 out of 10
Reasonable for current scale. Blocking filesystem operations in async commands and unbounded directory walks are latent risks.

Distribution Readiness: 2 out of 10
No signing, no updater, no installer customization. CI never builds the Tauri bundle.


Top 10 Risks

1. shell:allow-spawn grants unrestricted process spawning from the webview, enabling full remote code execution if the webview is compromised.
2. create_pty_session accepts an arbitrary command string with no allowlist, enabling arbitrary command execution via IPC.
3. list_directory and analyze_code_quality accept arbitrary filesystem paths, enabling full system enumeration.
4. No code signing is configured for any platform. OS warnings, antivirus flags, and binary tampering risk.
5. No cargo audit or pnpm audit in CI. Supply-chain vulnerabilities go undetected.
6. CI never runs pnpm tauri build. Integration failures between frontend and backend are invisible.
7. GitHub issue content is injected directly into Claude CLI prompts. Prompt injection vector.
8. Blocking std::process::Command and std::fs calls inside async functions can starve the Tokio runtime.
9. No logging framework anywhere in the Rust backend. Zero observability for security events or errors.
10. The production CSP includes http://localhost:1420 in connect-src, an unnecessary attack surface expansion.


Top 10 Quick Wins

1. Remove shell:allow-spawn from capabilities/default.json to eliminate the top RCE vector.
2. Add a command allowlist to create_pty_session restricting to claude and codex.
3. Add cargo audit and pnpm audit steps to CI to catch known CVEs immediately.
4. Remove http://localhost:1420 from the production CSP.
5. Drop the async keyword from get_git_branch, get_git_status, and list_directory so Tauri uses its blocking thread pool correctly.
6. Replace the hardcoded projectPath default with an empty string or CWD resolution.
7. Add object-src none, base-uri self, and form-action self to the CSP.
8. Add bounds checking to iso_to_epoch for month range 1 to 12 and day at least 1 to prevent a panic.
9. Add a global unhandledrejection listener in main.tsx.
10. Add the tracing crate with warn-level logging on suppressed errors and info-level logging on security-relevant commands.


SECTION 1: ARCHITECTURE OVERVIEW

1.1 High-Level System Layout

The application is a Tauri v2 desktop app with two layers:

Frontend Layer: React 19 with TypeScript, Zustand for state management, Tailwind CSS for styling, and xterm.js for terminal emulation. All code resides in the src/ directory.

Backend Layer: Rust, organized into commands/ (22 IPC command functions), claude/ (CLI integration helpers), and lib.rs (app builder and command registration). All code resides in src-tauri/src/.

The frontend communicates with the backend via Tauri's IPC bridge using invoke() for request-response calls and listen() for event streaming (PTY output).

1.2 Data Flow

Step 1: React component calls invoke("command_name", parameters) or a wrapper function from src/lib/tauri.ts.
Step 2: Tauri deserializes JSON parameters into typed Rust structures via serde.
Step 3: Rust command function executes (spawns CLI process, reads filesystem, calls GitHub API via reqwest).
Step 4: Result is serialized back to JSON and returned to the frontend.
Step 5: For PTY sessions, output streams asynchronously via app_handle.emit("pty:output") events that the frontend listens to.

1.3 Separation of Concerns

Generally good. Views are routed via the AppView union type in appStore. Stores are independent Zustand slices. Rust commands are organized by domain into separate modules.

Areas of tight coupling:
- TerminalPane.tsx, FileExplorer.tsx, and memoryStore.ts call invoke() directly instead of using the centralized tauri.ts wrapper.
- Three status bar components independently poll their respective status endpoints with no shared orchestration.
- VibeArchitectView.tsx embeds a third-party URL directly with no abstraction layer.

1.4 Anti-Patterns

Hardcoded developer path: layoutStore.ts line 26 ships "D:\projects\PacketCode" as the default projectPath.
No environment distinction: Zero references to import.meta.env.DEV or import.meta.env.PROD across the entire frontend.
Async functions that block: get_git_branch, get_git_status, list_directory, and analyze_code_quality are declared as async functions but call synchronous standard library APIs that block the Tokio worker thread.


SECTION 2: TAURI-SPECIFIC AUDIT

2.1 IPC and Command Safety

There are 22 registered Tauri command functions in lib.rs lines 16 through 52. The critical finding is that create_pty_session (pty.rs line 58) accepts an arbitrary command string parameter with no allowlist validation. This function spawns a child process using portable-pty's CommandBuilder with whatever binary name the frontend provides.

All commands return Result with String error types. Error propagation is generally good with descriptive error messages. Three sites in pty.rs silently discard errors using let _ = patterns.

One confirmed panic path exists in statusline.rs line 306 where array indexing occurs without validating the month is in range 1 through 12.

2.2 Security Model

The capabilities/default.json file contains the critical finding: shell:allow-spawn on line 17 with no scoping object. This grants unrestricted process spawning from webview JavaScript. The scoped shell:allow-execute block (lines 21 through 39) restricts commands to claude, git, and where, but sets args to true, permitting arbitrary arguments.

The CSP in tauri.conf.json line 26 includes style-src unsafe-inline (required for Tailwind but weakens XSS mitigation), http://localhost:1420 in connect-src (dev artifact), and third-party specs-gen.vercel.app in both connect-src and frame-src.

No devtools restriction is explicitly configured for production builds.

2.3 Rust Backend Quality

Error handling is consistent throughout. The map_err pattern is used across all commands. No unwrap() calls in production paths. One expect() at lib.rs line 54 for the Tauri runner (acceptable for fatal startup). Poisoned mutex recovery pattern used correctly in statusline.rs line 561.

No logging framework is used anywhere. Zero log statements. The tracing, log, and env_logger crates are absent from Cargo.toml.

Three async functions (git.rs, fs.rs, code_quality.rs) perform blocking I/O on the Tokio runtime. No unsafe blocks exist anywhere.

2.4 Frontend Quality

No XSS vectors found. No dangerouslySetInnerHTML, no eval(), no dynamic script injection. Markdown rendering uses react-markdown without rehype-raw.

State management uses clean Zustand patterns with TypeScript interfaces. localStorage persistence has no size limits and fails silently when quota is exceeded.

Error boundaries are implemented at three levels: app root, view content, and per-module. However, no global unhandledrejection handler exists.


SECTION 3: SECURITY REVIEW

3.1 Threat Model

PacketCode is a desktop IDE that spawns child processes, accesses the filesystem, makes HTTP requests to GitHub, and embeds a third-party iframe. The primary threats are:
- Compromised webview via dependency supply chain, XSS, or malicious iframe content.
- Local privilege escalation via unrestricted command execution.
- Data exfiltration via filesystem enumeration and network access.
- Prompt injection via GitHub issue content passed to AI CLI tools.

3.2 Critical Injection Surfaces

PTY command spawning: The command parameter in create_pty_session (pty.rs line 64) accepts any string. Combined with shell:allow-spawn in the capabilities, this creates a direct path from webview JavaScript to arbitrary command execution with the user's full operating system privileges.

Filesystem enumeration: list_directory (fs.rs line 22) and analyze_code_quality (code_quality.rs line 282) accept any filesystem path. The hidden-file filter in fs.rs line 37 explicitly allows .env and .env.local files, enabling secret discovery.

Prompt injection: GitHub issue titles and bodies are interpolated directly into Claude CLI prompts (github.rs lines 227 through 244) without sanitization.

3.3 Secrets Handling

The GitHub personal access token is stored as a plain String in process memory (github.rs line 6). It is not persisted to disk (good) but is not encrypted in memory and not zeroized when cleared. Legacy tokens in localStorage are properly migrated and removed.

No other secrets are handled by the application. Claude and Codex CLI tools manage their own authentication externally.

3.4 Supply Chain

No cargo audit or pnpm audit runs in the CI pipeline. No Dependabot or Renovate is configured. All Rust crate versions use major-only specifiers. The portable-pty crate handles process spawning directly and any vulnerability in it would be critical.


SECTION 4: RELIABILITY AND STABILITY

4.1 Panic Paths

Three panic paths were identified:
- statusline.rs line 306: Array index out of bounds if a malformed JSONL timestamp has a month value greater than 13.
- lib.rs line 54: The expect() call panics if the Tauri runtime fails to initialize.
- code_quality.rs line 251: Unbounded recursive directory walk could stack overflow on symlink cycles.

4.2 Error Boundaries

React ErrorBoundary components wrap the app at three levels. However, they only catch synchronous render errors. No global window.onerror or unhandledrejection handler is installed, so async errors in store actions and effect callbacks are silently lost.

4.3 State Corruption Risk

localStorage persistence uses empty catch blocks. If the 5 to 10 megabyte quota is exceeded, writes fail silently and data is lost. No data versioning or migration framework exists.

4.4 Data Persistence

All persistent state uses localStorage under packetcode: keys. No SQLite, no filesystem storage, no Tauri app data directory. This creates a size ceiling and makes data migration between versions impossible.


SECTION 5: PERFORMANCE

5.1 Startup Time

No lazy loading of views. All view components are imported eagerly. No code splitting configured. react-syntax-highlighter loads all language grammars by default.

5.2 Blocking Operations

git.rs: Blocking std::process::Command::output() in async context, called every few seconds via polling.
fs.rs: Blocking std::fs::read_dir in async context.
code_quality.rs: Blocking filesystem walk and file reads in async context. On large codebases this could block for seconds.

5.3 Memory Risks

Unbounded directory walk in code_quality.rs collects all files before processing. No size validation on any IPC string parameter.


SECTION 6: TESTING AND QUALITY GATES

6.1 Test Coverage

Rust: 5 unit tests in code_quality.rs. No tests for pty.rs, git.rs, github.rs, memory.rs, insights.rs, ideation.rs, spec.rs, fs.rs, or statusline.rs.
Frontend: Zero tests. No test runner in dependencies. No test files.
Integration: None. No end-to-end tests.

6.2 CI Pipeline

Two jobs run on Ubuntu only:
- Frontend: pnpm lint and pnpm build
- Backend: cargo test

Missing: Tauri build, multi-platform matrix, dependency auditing, SAST tools, coverage reporting, license scanning, artifact signing.


SECTION 7: DISTRIBUTION AND DEVOPS

7.1 Build Configuration

The bundle targets setting is "all", building every installer format. No platform-specific customization exists. The Vite dev server can bind to any address when TAURI_DEV_HOST is set.

7.2 Code Signing

Not configured for any platform. This is a distribution blocker for Windows (SmartScreen) and macOS (Gatekeeper).

7.3 Auto-Update

Not configured. No tauri-plugin-updater dependency. No update endpoint. Users must manually download new versions.

7.4 Cross-Platform

CI runs only on Ubuntu. Windows-specific PTY behavior and path handling are untested. The .cmd wrapper resolution logic in pty.rs is correctly gated behind cfg!(windows).


SECTION 8: FINDINGS (PRIORITIZED)

Finding 01
Severity: Critical
Title: Unrestricted Process Spawning via shell:allow-spawn
Impact: Full remote code execution if webview is compromised
Evidence: capabilities/default.json line 17
Why It Matters: This bypasses the scoped execute allowlist entirely. Any JavaScript running in the webview can spawn any process on the host.
Recommendation: Remove shell:allow-spawn entirely
Owner: Security / Backend lead
Effort: Small (less than 1 hour)
Risk of Change: Low

Finding 02
Severity: Critical
Title: Arbitrary Command Execution via create_pty_session
Impact: Any webview code can execute any binary on the host
Evidence: pty.rs lines 58 through 66, command parameter with no allowlist
Why It Matters: Even if Finding 01 is fixed, this IPC command accepts any command string from the frontend
Recommendation: Add a command allowlist restricting to known CLI tools
Owner: Backend lead
Effort: Small (less than 1 hour)
Risk of Change: Low

Finding 03
Severity: Critical
Title: No Code Signing Configured
Impact: OS warnings block installation, binaries can be tampered with
Evidence: tauri.conf.json lines 29 through 39, no signing configuration
Why It Matters: Distribution blocker for any production release on Windows or macOS
Recommendation: Obtain signing certificates and configure in bundle settings
Owner: DevOps
Effort: Large (1 to 2 weeks)
Risk of Change: None

Finding 04
Severity: High
Title: Arbitrary Path Traversal in list_directory
Impact: Full filesystem enumeration, .env secret discovery
Evidence: fs.rs line 22, no path scope validation
Why It Matters: Enables reconnaissance for further attacks and secret discovery
Recommendation: Validate paths are within the project workspace
Owner: Backend lead
Effort: Small (1 to 2 hours)
Risk of Change: Low

Finding 05
Severity: High
Title: Arbitrary Path Traversal in analyze_code_quality
Impact: Recursive file reading of any directory
Evidence: code_quality.rs line 282, no path scope validation
Why It Matters: Data exfiltration of source code from any accessible directory
Recommendation: Add workspace path validation and recursion depth limit
Owner: Backend lead
Effort: Small (2 to 3 hours)
Risk of Change: Low

Finding 06
Severity: High
Title: No Dependency Vulnerability Scanning in CI
Impact: Known CVEs go undetected
Evidence: ci.yml has no audit steps
Why It Matters: Supply chain attacks are the number one threat vector for desktop apps
Recommendation: Add cargo audit and pnpm audit to CI
Owner: DevOps
Effort: Small (30 minutes)
Risk of Change: None

Finding 07
Severity: High
Title: CI Never Builds the Tauri Application
Impact: Integration failures are invisible
Evidence: ci.yml only runs frontend build and Rust tests
Why It Matters: Type mismatches, missing command registrations, and packaging errors are not caught
Recommendation: Add pnpm tauri build to CI with platform matrix
Owner: DevOps
Effort: Medium (half day)
Risk of Change: None

Finding 08
Severity: High
Title: Blocking I/O on Tokio Async Runtime
Impact: Tokio worker thread starvation under load
Evidence: git.rs lines 4 and 27, fs.rs line 22, code_quality.rs line 282
Why It Matters: Polled functions block Tokio worker threads, freezing IPC processing under load
Recommendation: Remove async keyword or use spawn_blocking
Owner: Backend lead
Effort: Small (1 hour)
Risk of Change: Low

Finding 09
Severity: High
Title: shell:allow-execute Permits Arbitrary Arguments
Impact: Dangerous git or claude flags can be passed from webview
Evidence: capabilities/default.json line 27, args set to true
Why It Matters: Expands attack surface even with the command allowlist in place
Recommendation: Replace with regex argument validators
Owner: Security / Backend lead
Effort: Medium (2 to 4 hours)
Risk of Change: Medium

Finding 10
Severity: Medium
Title: No Logging Framework in Rust Backend
Impact: Zero observability for security events and errors
Evidence: No tracing or log crate in Cargo.toml
Why It Matters: Cannot diagnose issues, detect abuse, or audit command invocations
Recommendation: Add tracing with audit logging
Owner: Backend lead
Effort: Medium (half day)
Risk of Change: None

Finding 11
Severity: Medium
Title: Prompt Injection via GitHub Issue Content
Impact: Malicious issues can manipulate Claude analysis output
Evidence: github.rs lines 227 through 244
Why It Matters: Attacker-controlled content is interpolated directly into AI prompts
Recommendation: Sanitize issue content with delimiter-based injection mitigation
Owner: Backend lead
Effort: Small (1 to 2 hours)
Risk of Change: Low

Finding 12
Severity: Medium
Title: Localhost Dev URL in Production CSP
Impact: Unnecessary attack surface expansion
Evidence: tauri.conf.json line 26
Why It Matters: Dev artifact that allows connections to local services in production
Recommendation: Remove http://localhost:1420 from production CSP
Owner: Frontend lead
Effort: Small (15 minutes)
Risk of Change: None

Finding 13
Severity: Medium
Title: Hardcoded Developer Path as Default
Impact: Invalid path on fresh installs on other machines
Evidence: layoutStore.ts line 26
Why It Matters: Leaks developer environment and causes confusing errors
Recommendation: Default to empty string or CWD resolution
Owner: Frontend lead
Effort: Small (15 minutes)
Risk of Change: None

Finding 14
Severity: Medium
Title: No Auto-Update Mechanism
Impact: No way to push security patches to users
Evidence: No tauri-plugin-updater in dependencies
Why It Matters: Critical for desktop apps that handle sensitive operations
Recommendation: Add updater with signed manifests and GitHub Releases endpoint
Owner: DevOps
Effort: Medium (1 to 2 days)
Risk of Change: Low

Finding 15
Severity: Medium
Title: Panic Risk in iso_to_epoch
Impact: Malformed timestamp crashes the Tauri process
Evidence: statusline.rs lines 303 through 311
Why It Matters: Status line files from external processes may contain unexpected data
Recommendation: Add range validation for month and day values
Owner: Backend lead
Effort: Small (15 minutes)
Risk of Change: None

Finding 16
Severity: Medium
Title: localStorage Persistence Without Size Limits
Impact: Silent data loss when quota is exceeded
Evidence: issueStore.ts lines 107 through 111, empty catch blocks
Why It Matters: Issue data, memory maps, and chat histories grow indefinitely
Recommendation: Add storage monitoring and data pruning
Owner: Frontend lead
Effort: Medium (half day)
Risk of Change: Low

Finding 17
Severity: Medium
Title: No Global Unhandled Rejection Handler
Impact: Async errors silently lost
Evidence: main.tsx has no unhandledrejection listener
Why It Matters: Bugs in async code paths produce no user-visible feedback
Recommendation: Add global handler with error logging
Owner: Frontend lead
Effort: Small (30 minutes)
Risk of Change: None

Finding 18
Severity: Medium
Title: Iframe Sandbox Negated
Impact: Compromised third-party site could access localStorage
Evidence: VibeArchitectView.tsx line 42
Why It Matters: allow-scripts plus allow-same-origin combination weakens sandbox protections
Recommendation: Verify origin isolation, consider proxying
Owner: Frontend lead
Effort: Medium
Risk of Change: Medium

Finding 19
Severity: Medium
Title: CI Only Runs on Ubuntu
Impact: Platform-specific issues untested
Evidence: ci.yml lines 12 and 30
Why It Matters: Windows and macOS behavior differences are never validated
Recommendation: Add OS matrix strategy
Owner: DevOps
Effort: Small (1 hour)
Risk of Change: None

Finding 20
Severity: Low
Title: GitHub Token Not Zeroized in Memory
Impact: Token remains in memory after clear
Evidence: github.rs line 64
Why It Matters: Same-user processes can read another process memory on most OSes
Recommendation: Use zeroize crate and consider OS keychain
Owner: Backend lead
Effort: Small (1 hour)
Risk of Change: None


SECTION 9: ROADMAP

Phase 1: Now (0 to 2 weeks) - Critical Security and Stability

Priority 1: Remove shell:allow-spawn from capabilities (Finding 01, Small effort)
Priority 2: Add command allowlist to create_pty_session (Finding 02, Small effort)
Priority 3: Add cargo audit and pnpm audit to CI (Finding 06, Small effort)
Priority 4: Remove localhost from production CSP (Finding 12, Small effort)
Priority 5: Fix iso_to_epoch bounds checking (Finding 15, Small effort)
Priority 6: Add path validation to list_directory and analyze_code_quality (Findings 04 and 05, Small effort)
Priority 7: Replace hardcoded projectPath default (Finding 13, Small effort)
Priority 8: Fix async blocking in git.rs, fs.rs, code_quality.rs (Finding 08, Small effort)
Priority 9: Add global unhandledrejection handler (Finding 17, Small effort)

Phase 2: Next (2 to 6 weeks) - Defense in Depth and Observability

Priority 10: Scope shell:allow-execute args with validators (Finding 09, Medium effort)
Priority 11: Add tracing logging framework (Finding 10, Medium effort)
Priority 12: Sanitize GitHub issue content in prompts (Finding 11, Small effort)
Priority 13: Add Tauri build to CI with platform matrix (Findings 07 and 19, Medium effort)
Priority 14: Harden CSP with missing directives (Finding 12, Small effort)
Priority 15: Investigate iframe origin isolation (Finding 18, Medium effort)
Priority 16: Add input size limits to IPC commands (Medium effort)

Phase 3: Later (6 to 12 weeks) - Distribution and Production Readiness

Priority 17: Configure code signing for Windows and macOS (Finding 03, Large effort)
Priority 18: Implement auto-updater (Finding 14, Medium effort)
Priority 19: Migrate persistence from localStorage to Tauri filesystem storage (Finding 16, Medium effort)
Priority 20: Add data versioning and migration framework (Medium effort)
Priority 21: Add frontend test suite with Vitest (Large effort)
Priority 22: Add integration and end-to-end tests (Large effort)
Priority 23: Use OS keychain for token storage (Finding 20, Small effort)
Priority 24: Implement code splitting and lazy loading (Medium effort)
Priority 25: Add crash reporter (Medium effort)


Remediation Phase Documents

Detailed remediation plans with code samples, evidence, and testing checklists are available in the following documents:

Phase 01: Process Execution Lockdown (phase-01-process-execution-lockdown.md)
Phase 02: Filesystem Path Traversal (phase-02-filesystem-path-traversal.md)
Phase 03: CSP and Web Security Hardening (phase-03-csp-web-security-hardening.md)
Phase 04: Async Runtime and Stability (phase-04-async-runtime-stability.md)
Phase 05: CI/CD Pipeline Hardening (phase-05-cicd-pipeline-hardening.md)
Phase 06: Observability and Logging (phase-06-observability-logging.md)
Phase 07: Input Validation and Prompt Injection (phase-07-input-validation-prompt-injection.md)
Phase 08: Frontend Quality and State Management (phase-08-frontend-quality-state.md)
Phase 09: Code Signing and Distribution (phase-09-code-signing-distribution.md)
Phase 10: Testing, Data Migration, and Long-Term Quality (phase-10-testing-data-longterm.md)


END OF REPORT
