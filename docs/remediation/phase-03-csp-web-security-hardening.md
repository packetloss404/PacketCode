# Phase 3: CSP & Web Security Hardening

**Priority:** P1 — This Week
**Timeline:** 5–8 days
**Effort:** Small to Medium (4–6 hours total)
**Risk Level:** Medium
**Owners:** Frontend Lead, Security Lead

---

## Overview

The Content Security Policy (CSP) and web security configuration have several gaps that widen the attack surface unnecessarily. While no active exploit paths were found through these gaps alone, they reduce the defense-in-depth posture of the application. In a desktop app that spawns processes and accesses the filesystem, every layer of defense matters.

This phase tightens the CSP, addresses iframe sandbox concerns, and removes development artifacts from the production security configuration.

---

## Finding F-12: Localhost Dev URL in Production CSP

### Severity: Medium

### Description

The CSP defined in `src-tauri/tauri.conf.json` (line 26) includes `http://localhost:1420` in the `connect-src` directive. This is the Vite dev server URL, which is only relevant during development. In production builds, the frontend is served from Tauri's internal scheme, not localhost.

### Evidence

```json
// src-tauri/tauri.conf.json, line 26
"csp": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' asset: https://asset.localhost; connect-src ipc: http://ipc.localhost http://localhost:1420 https://api.github.com https://specs-gen.vercel.app; frame-src https://specs-gen.vercel.app"
```

The `http://localhost:1420` entry is a dev artifact that was never removed.

### Impact

- A malicious local service listening on port 1420 could receive connections from the PacketCode webview
- If a user has another application running on that port, PacketCode could inadvertently interact with it
- Weakens the CSP without providing any production benefit

### Remediation

Remove `http://localhost:1420` from the CSP `connect-src` directive. If the dev server URL is needed during development, use Tauri's environment-aware configuration or maintain separate dev/prod CSP strings.

**After:**
```json
"csp": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' asset: https://asset.localhost; connect-src ipc: http://ipc.localhost https://api.github.com https://specs-gen.vercel.app; frame-src https://specs-gen.vercel.app"
```

### Risk of Change

None in production. Verify that `pnpm tauri dev` still works correctly — Tauri may inject the dev URL automatically during development regardless of the configured CSP.

---

## Finding F-12b: Missing CSP Hardening Directives

### Severity: Low

### Description

The CSP relies on `default-src 'self'` as a fallback for unlisted directives. While this is a reasonable baseline, several directives should be explicitly set to harden against specific attack vectors.

### Evidence

Missing from the current CSP:
- `object-src` — not set, falls back to `'self'`. Should be `'none'` to prevent Flash/Java plugin embeds.
- `base-uri` — not set, falls back to `'self'`. Should be `'self'` to prevent `<base>` tag hijacking that redirects relative URLs.
- `form-action` — not set, falls back to `'self'`. Should be `'self'` to prevent forms from submitting data to external origins.

### Impact

- `object-src` without explicit `'none'` allows plugin-based attacks (increasingly rare but still a vector)
- Missing `base-uri` could allow a `<base>` tag injection to redirect relative URL resolution
- Missing `form-action` could allow form data exfiltration to external origins

### Remediation

Add the missing directives to the CSP string:

```json
"csp": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' asset: https://asset.localhost; connect-src ipc: http://ipc.localhost https://api.github.com https://specs-gen.vercel.app; frame-src https://specs-gen.vercel.app; object-src 'none'; base-uri 'self'; form-action 'self'"
```

### Risk of Change

None. These directives restrict capabilities that the application does not use.

---

## Finding F-12c: `style-src 'unsafe-inline'` Weakens CSP

### Severity: Medium (Accepted Risk)

### Description

The CSP includes `'unsafe-inline'` in the `style-src` directive. This allows inline `<style>` tags and `style=""` attributes, which weakens XSS mitigation. Injected inline styles can exfiltrate data via CSS selectors or perform UI redressing attacks.

### Evidence

```
style-src 'self' 'unsafe-inline'
```

This is a common and often necessary concession for applications using CSS-in-JS frameworks or Tailwind CSS, which generates utility classes that may be applied as inline styles at runtime.

### Impact

- If an attacker can inject HTML but not scripts, they can still inject `<style>` elements to perform CSS-based data exfiltration or UI manipulation
- This is a known trade-off, not a fixable vulnerability in most Tailwind/React setups

### Remediation

This is an **accepted risk** for the current tech stack. Document the trade-off. If Tailwind's runtime styles are fully static (generated at build time and included in CSS files), test removing `'unsafe-inline'` to see if the application still renders correctly. If nonce-based inline styles are feasible in the future, prefer that approach.

**Action:** Add a comment in the codebase documenting why `'unsafe-inline'` is required:

```json
// Note: 'unsafe-inline' in style-src is required for Tailwind CSS runtime styles.
// This is an accepted trade-off. See docs/remediation/phase-03.
```

### Risk of Change

If removed without testing, the application's styling may break. Test thoroughly before removing.

---

## Finding F-18: Iframe Sandbox Negated by `allow-scripts` + `allow-same-origin`

### Severity: Medium

### Description

The `VibeArchitectView.tsx` component embeds a third-party URL (`https://specs-gen.vercel.app`) in an iframe with a sandbox attribute that includes both `allow-scripts` and `allow-same-origin`. This combination effectively negates most sandbox protections.

### Evidence

```tsx
// src/components/views/VibeArchitectView.tsx, lines 37-43
<iframe
  src={VIBE_ARCHITECT_URL}
  className="flex-1 w-full border-none"
  title="Vibe Architect"
  allow="clipboard-read; clipboard-write"
  sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-clipboard-read allow-clipboard-write"
/>
```

The hardcoded URL:

```tsx
// src/components/views/VibeArchitectView.tsx, line 5
const VIBE_ARCHITECT_URL = "https://specs-gen.vercel.app";
```

### Impact

When `allow-scripts` and `allow-same-origin` are combined in a sandbox:
- The iframe can access its own origin's cookies and storage
- If the Tauri webview assigns the same origin to both the parent and the iframe (depends on webview implementation), the iframe could access the parent's `localStorage`, reading all persisted PacketCode data
- If `specs-gen.vercel.app` is compromised or its Vercel deployment is hijacked, malicious code could access PacketCode's stored data

### Investigation Required

Before remediating, determine:
1. What origin does the Tauri webview assign to the parent page? (Typically `tauri://localhost` or `https://tauri.localhost`)
2. Does the iframe at `https://specs-gen.vercel.app` share that origin? (Almost certainly not — it has its own HTTPS origin)
3. If origins differ, the sandbox concern is mitigated by the Same-Origin Policy itself

### Remediation

**Option A (preferred):** If the investigation confirms different origins, document this as an accepted risk. The iframe is sandboxed and the Same-Origin Policy prevents cross-origin access.

**Option B:** If origins could overlap, proxy the VibeArchitect content through the Rust backend:
- Add a Tauri command that fetches the content and returns it
- Render it in a sandboxed iframe with `srcdoc` instead of `src`
- Remove `allow-same-origin` from the sandbox

**Option C (minimal):** Remove `allow-same-origin` from the sandbox. Test whether the embedded application still functions. Some applications require `allow-same-origin` for their own internal storage access.

### Additional Consideration

The external URL `https://specs-gen.vercel.app` is a single point of failure. If this Vercel deployment goes down, is deleted, or changes significantly, the Vibe Architect feature breaks silently. Consider:
- Adding a connectivity check before rendering the iframe
- Providing a fallback message if the service is unreachable
- Documenting the dependency and monitoring its availability

### Risk of Change

Medium. Removing `allow-same-origin` may break the embedded application's functionality. Test thoroughly.

---

## Finding F-12d: Third-Party Origins in CSP

### Severity: Medium (Informational)

### Description

The CSP allows connections and frames from two external origins:
- `https://api.github.com` — expected and necessary for GitHub integration
- `https://specs-gen.vercel.app` — used for the Vibe Architect iframe

### Evidence

```
connect-src ... https://api.github.com https://specs-gen.vercel.app;
frame-src https://specs-gen.vercel.app
```

### Impact

If either external service is compromised:
- `api.github.com`: The app could receive malicious API responses. Mitigated by the fact that GitHub's API is a high-security target with its own protections.
- `specs-gen.vercel.app`: Could serve malicious iframe content. Partially mitigated by sandbox attributes (see F-18).

### Remediation

- **GitHub API:** No action needed. This is a trusted, high-security API endpoint.
- **Vercel app:** Consider pinning to a specific subpath if the application serves content from a known route (e.g., `https://specs-gen.vercel.app/app/`). Monitor the deployment for unexpected changes.

### Risk of Change

None for documentation-only actions.

---

## Testing Checklist

- [ ] Remove `http://localhost:1420` from CSP `connect-src`
- [ ] Add `object-src 'none'; base-uri 'self'; form-action 'self'` to CSP
- [ ] Verify `pnpm tauri dev` still works (dev server connectivity)
- [ ] Verify production build renders correctly with updated CSP
- [ ] Investigate Tauri webview origin assignment for iframe isolation
- [ ] Test VibeArchitectView iframe functionality with modified sandbox (if applicable)
- [ ] Document `style-src 'unsafe-inline'` as accepted risk
- [ ] Verify no inline scripts are used anywhere (they would be blocked by `script-src 'self'`)

---

## References

- `src-tauri/tauri.conf.json` — line 26 (CSP configuration)
- `src/components/views/VibeArchitectView.tsx` — lines 5, 37–43
- Tauri v2 Security documentation: CSP configuration
- MDN Web Docs: Content-Security-Policy directives
- MDN Web Docs: iframe sandbox attribute
