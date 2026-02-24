# Phase 9: Code Signing & Distribution Readiness

**Priority:** P3 — Next Quarter
**Timeline:** Weeks 6–10
**Effort:** Large (1–2 weeks total)
**Risk Level:** Critical (distribution blocker)
**Owners:** DevOps, Release Engineering, Project Lead

---

## Overview

PacketCode cannot be distributed to end users in its current state. The application is unsigned, has no auto-update mechanism, no installer customization, and no rollback strategy. Windows SmartScreen will warn users that the app is from an "unknown publisher" and may block execution entirely. macOS Gatekeeper will refuse to open the application without notarization. This phase establishes the complete distribution pipeline.

---

## Finding F-03: No Code Signing Configured

### Severity: Critical (Distribution Blocker)

### Description

There is no code signing configuration anywhere in the project. The `bundle` section of `tauri.conf.json` contains only icon paths and `"targets": "all"`. No Windows certificate, no macOS signing identity, and no Linux signing.

### Evidence

```json
// src-tauri/tauri.conf.json, lines 29-39
"bundle": {
  "active": true,
  "targets": "all",
  "icon": [
    "icons/32x32.png",
    "icons/128x128.png",
    "icons/128x128@2x.png",
    "icons/icon.icns",
    "icons/icon.ico"
  ]
}
```

No signing-related keys: `windows.certificateThumbprint`, `windows.signCommand`, `macOS.signingIdentity`, or notarization configuration.

### Impact

**Windows:**
- SmartScreen shows "Windows protected your PC" warning
- Some antivirus software flags unsigned executables as potentially unsafe
- Enterprise environments may block unsigned app installation via Group Policy
- The NSIS installer is unsigned — users see "Unknown publisher" in UAC prompts

**macOS:**
- Gatekeeper blocks unsigned/unnotarized apps with "cannot be opened because the developer cannot be verified"
- Users must manually bypass via System Preferences, which erodes trust
- App Store distribution is impossible without signing

**Linux:**
- No standard signing requirement, but unsigned AppImages lack trust indicators
- Package managers (apt, yum) require GPG-signed packages for repository distribution

### Remediation

#### Windows Code Signing

**Step 1: Obtain a code signing certificate**

Options:
- **EV (Extended Validation) certificate** — instant SmartScreen reputation. ~$300–500/year from DigiCert, Sectigo, or GlobalSign.
- **OV (Organization Validation) certificate** — builds reputation over time. ~$100–200/year.
- **Azure Trusted Signing** — Microsoft's cloud-based signing service. Pay-per-use.

**Step 2: Configure Tauri for Windows signing**

Add to `tauri.conf.json`:

```json
"bundle": {
  "active": true,
  "targets": ["nsis"],
  "windows": {
    "certificateThumbprint": "YOUR_CERT_THUMBPRINT",
    "digestAlgorithm": "sha256",
    "timestampUrl": "http://timestamp.digicert.com"
  }
}
```

Or use a custom sign command for cloud-based signing:

```json
"windows": {
  "signCommand": "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /sha1 THUMBPRINT \"%1\""
}
```

**Step 3: Add signing to CI**

```yaml
  release:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      # ... setup steps ...
      - name: Import certificate
        env:
          WINDOWS_CERTIFICATE: ${{ secrets.WINDOWS_CERTIFICATE }}
          WINDOWS_CERTIFICATE_PASSWORD: ${{ secrets.WINDOWS_CERTIFICATE_PASSWORD }}
        run: |
          $cert = [System.Convert]::FromBase64String($env:WINDOWS_CERTIFICATE)
          [System.IO.File]::WriteAllBytes("cert.pfx", $cert)
          certutil -f -p $env:WINDOWS_CERTIFICATE_PASSWORD -importpfx cert.pfx
      - name: Build and sign
        run: pnpm tauri build
```

#### macOS Code Signing & Notarization

**Step 1: Enroll in Apple Developer Program ($99/year)**

**Step 2: Create signing identity and notarization credentials**

**Step 3: Configure Tauri for macOS signing**

Add to `tauri.conf.json`:

```json
"bundle": {
  "macOS": {
    "signingIdentity": "Developer ID Application: Your Name (TEAM_ID)",
    "entitlements": "Entitlements.plist"
  }
}
```

Set environment variables for notarization:

```yaml
  release-macos:
    runs-on: macos-latest
    env:
      APPLE_CERTIFICATE: ${{ secrets.APPLE_CERTIFICATE }}
      APPLE_CERTIFICATE_PASSWORD: ${{ secrets.APPLE_CERTIFICATE_PASSWORD }}
      APPLE_SIGNING_IDENTITY: ${{ secrets.APPLE_SIGNING_IDENTITY }}
      APPLE_ID: ${{ secrets.APPLE_ID }}
      APPLE_PASSWORD: ${{ secrets.APPLE_APP_SPECIFIC_PASSWORD }}
      APPLE_TEAM_ID: ${{ secrets.APPLE_TEAM_ID }}
```

#### Linux Signing

For AppImage distribution, GPG signing is recommended but not required:

```bash
gpg --detach-sign --armor PacketCode.AppImage
```

### Cost Estimate

| Item | Cost | Frequency |
|------|------|-----------|
| Windows EV code signing certificate | $300–500 | Annual |
| Apple Developer Program | $99 | Annual |
| CI/CD compute for multi-platform builds | Variable | Per-build |

### Risk of Change

None. Code signing is an additive build step that does not change application behavior.

---

## Finding F-14: No Auto-Update Mechanism

### Severity: Medium

### Description

There is no auto-update mechanism configured. The `tauri-plugin-updater` crate is not in `Cargo.toml`, there is no updater configuration in `tauri.conf.json`, and no update server endpoint exists. Users must manually check for updates, download new versions, and reinstall.

### Impact

- Security patches cannot be pushed to users
- Users may run outdated versions with known vulnerabilities
- No way to communicate critical updates
- Manual distribution is error-prone and doesn't scale

### Remediation

**Step 1: Add the updater plugin**

```toml
# src-tauri/Cargo.toml
[dependencies]
tauri-plugin-updater = "2"
```

```json
// package.json
"@tauri-apps/plugin-updater": "^2.0.0"
```

**Step 2: Configure the updater in `tauri.conf.json`**

```json
"plugins": {
  "updater": {
    "endpoints": [
      "https://github.com/YOUR_ORG/PacketCode/releases/latest/download/latest.json"
    ],
    "pubkey": "YOUR_PUBLIC_KEY_HERE"
  }
}
```

**Step 3: Generate signing keys**

```bash
pnpm tauri signer generate -w ~/.tauri/PacketCode.key
```

Store the private key as a CI secret (`TAURI_SIGNING_PRIVATE_KEY`). Embed the public key in the updater config.

**Step 4: Register the plugin in `lib.rs`**

```rust
tauri::Builder::default()
    .plugin(tauri_plugin_updater::Builder::new().build())
    // ... other plugins
```

**Step 5: Add update check to frontend**

```typescript
import { check } from "@tauri-apps/plugin-updater";

async function checkForUpdates() {
  const update = await check();
  if (update) {
    // Show update notification to user
    await update.downloadAndInstall();
  }
}
```

**Step 6: Configure CI to publish releases**

Create a release workflow that:
1. Bumps the version
2. Builds signed installers for all platforms
3. Creates a GitHub Release with the installers
4. Generates the `latest.json` update manifest

### Risk of Change

Low. The updater is an additive feature. The main risk is in key management — losing the signing key would require users to manually update.

---

## Finding F-03b: Bundle Configuration Gaps

### Severity: Low

### Description

The bundle configuration has several gaps that affect installer quality and user experience:

1. `"targets": "all"` builds every installer format, including formats that may not be tested
2. No NSIS-specific configuration (install path, start menu group, per-user vs per-machine)
3. No DMG-specific configuration (background image, window size, app icon position)
4. No installer license agreement
5. No file associations

### Evidence

```json
// src-tauri/tauri.conf.json, lines 29-39
"bundle": {
  "active": true,
  "targets": "all",
  "icon": [...]
}
```

### Remediation

**Specify target formats:**

```json
"bundle": {
  "active": true,
  "targets": ["nsis", "dmg", "appimage"],
  "icon": [...]
}
```

**Add NSIS configuration (Windows):**

```json
"bundle": {
  "windows": {
    "nsis": {
      "installMode": "both",
      "displayLanguageSelector": false,
      "startMenuFolder": "PacketCode",
      "headerImage": "icons/installer-header.bmp",
      "sidebarImage": "icons/installer-sidebar.bmp"
    }
  }
}
```

**Add DMG configuration (macOS):**

```json
"bundle": {
  "macOS": {
    "dmg": {
      "appPosition": { "x": 180, "y": 170 },
      "applicationFolderPosition": { "x": 480, "y": 170 },
      "windowSize": { "width": 660, "height": 400 }
    }
  }
}
```

### Risk of Change

Low. Installer configuration changes only affect the distribution packaging, not the application itself.

---

## Finding F-03c: No Rollback Strategy

### Severity: Medium

### Description

There is no documented or implemented rollback strategy. If a release introduces a critical bug:
- There is no way to automatically revert to the previous version
- No versioned data persistence means data may be incompatible between versions
- No mechanism to force-downgrade users

### Remediation

**Step 1: Maintain previous release artifacts**

Keep at least the last 3 releases available for manual download from GitHub Releases.

**Step 2: Document manual rollback procedure**

Create user-facing documentation explaining how to:
1. Uninstall the current version
2. Download a specific previous version
3. Install the previous version
4. Verify data compatibility

**Step 3: Consider version gating in auto-updater**

The Tauri updater can be configured to only update to specific versions. In an emergency, point the update manifest to a known-good version instead of the latest.

### Risk of Change

None. This is process and documentation, not code changes.

---

## Testing Checklist

- [ ] Obtain Windows code signing certificate
- [ ] Enroll in Apple Developer Program
- [ ] Configure Windows signing in `tauri.conf.json`
- [ ] Configure macOS signing and notarization
- [ ] Add `tauri-plugin-updater` to Rust and npm dependencies
- [ ] Generate update signing key pair
- [ ] Configure updater endpoint (GitHub Releases)
- [ ] Register updater plugin in `lib.rs`
- [ ] Add update check UI in frontend
- [ ] Create release CI workflow with multi-platform builds
- [ ] Specify explicit bundle targets (remove `"all"`)
- [ ] Add NSIS installer configuration
- [ ] Test signed Windows installer (SmartScreen should not warn)
- [ ] Test signed macOS DMG (Gatekeeper should not block)
- [ ] Test auto-update flow end-to-end
- [ ] Document manual rollback procedure
- [ ] Store signing secrets in CI secret management

---

## References

- `src-tauri/tauri.conf.json` — lines 29–39
- `src-tauri/Cargo.toml` — dependencies
- `package.json` — npm dependencies
- Tauri v2 documentation: Code Signing
- Tauri v2 documentation: Updater Plugin
- Tauri v2 documentation: NSIS Configuration
- Apple Developer documentation: Notarizing macOS software
- Microsoft documentation: Authenticode code signing
