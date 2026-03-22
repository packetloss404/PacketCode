# PacketCode Funnel System — Full Specification & Mockup

> A complete spec for an automated marketing funnel system with GUI, content generation pipeline, multi-platform distribution, and analytics — buildable with PacketCode's existing tech stack.

---

## Table of Contents

1. [System Overview](#system-overview)
2. [Architecture](#architecture)
3. [GUI Mockups](#gui-mockups)
4. [Module Specifications](#module-specifications)
5. [Content Generation Pipeline](#content-generation-pipeline)
6. [Social Media Distribution Engine](#social-media-distribution-engine)
7. [Funnel Builder & Manager](#funnel-builder--manager)
8. [Analytics Dashboard](#analytics-dashboard)
9. [Integration Layer](#integration-layer)
10. [Data Models](#data-models)
11. [Tech Stack](#tech-stack)
12. [Implementation Plan](#implementation-plan)
13. [Cost Analysis](#cost-analysis)

---

## System Overview

### What Is This?

A self-hosted marketing automation GUI that:
1. **Generates** content (video clips, social posts, images) from raw material (screen recordings, blog posts, changelogs)
2. **Distributes** content across TikTok, Reddit, Instagram, Facebook, YouTube, X/Twitter, LinkedIn, Threads
3. **Manages** marketing funnels visually (awareness → activation → conversion → advocacy)
4. **Tracks** analytics across the entire funnel (PostHog integration)
5. **Automates** scheduling, posting, and email sequences

### Why Build This?

- Commercial funnel tools (GoHighLevel, ClickFunnels) cost $97-497/month and are designed for marketers, not developers
- No existing tool combines content generation + distribution + funnel management + analytics for dev tool marketing
- PacketCode's existing stack (React, TypeScript, Zustand, Tailwind, Tauri) can power this as a module or standalone app
- Self-hosted = full data ownership, no monthly SaaS fees beyond API costs

### Design Principles

- **Developer-first UX** — keyboard shortcuts, JSON config, CLI-friendly
- **Automation-heavy** — minimize manual steps; human-in-the-loop only for quality control
- **Open source integrations** — prefer Postiz, PostHog, n8n over proprietary tools
- **Modular** — each module works independently; compose as needed

---

## Architecture

### High-Level System Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                     FUNNEL SYSTEM GUI (React + Tailwind)            │
│                                                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐   │
│  │ Content  │  │ Schedule │  │  Funnel  │  │    Analytics     │   │
│  │ Studio   │  │ & Post   │  │ Builder  │  │    Dashboard     │   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────────┬─────────┘   │
│       │              │             │                  │             │
└───────┼──────────────┼─────────────┼──────────────────┼─────────────┘
        │              │             │                  │
┌───────┼──────────────┼─────────────┼──────────────────┼─────────────┐
│       ▼              ▼             ▼                  ▼             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐   │
│  │ AI APIs  │  │ Social   │  │  Funnel  │  │   PostHog API    │   │
│  │ (Claude, │  │ Media    │  │  State   │  │   (Analytics)    │   │
│  │ OpenAI,  │  │ APIs     │  │  Store   │  │                  │   │
│  │ HeyGen)  │  │ (Late/   │  │ (JSON/   │  │                  │   │
│  │          │  │ Postiz)  │  │  SQLite) │  │                  │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────────┘   │
│                                                                     │
│                    BACKEND (Tauri/Rust or Node.js)                   │
└─────────────────────────────────────────────────────────────────────┘
        │              │             │                  │
        ▼              ▼             ▼                  ▼
   ┌─────────┐  ┌───────────┐  ┌─────────┐  ┌──────────────────┐
   │ Opus    │  │ TikTok   │  │ Resend  │  │ PostHog          │
   │ Clip    │  │ Reddit   │  │ (Email) │  │ (self-hosted)    │
   │ CapCut  │  │ Instagram│  │         │  │                  │
   │ HeyGen  │  │ Facebook │  │         │  │                  │
   │ Zebracat│  │ YouTube  │  │         │  │                  │
   │         │  │ X/Twitter│  │         │  │                  │
   │         │  │ LinkedIn │  │         │  │                  │
   │         │  │ Threads  │  │         │  │                  │
   └─────────┘  └───────────┘  └─────────┘  └──────────────────┘
```

### Data Flow

```
Raw Material (screen recording, blog post, changelog)
        │
        ▼
┌─────────────────────────┐
│   Content Studio        │
│   - AI generates copy   │
│   - Video auto-clipping │
│   - Image generation    │
│   - Human review queue  │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│   Content Library       │
│   - Approved assets     │
│   - Platform variants   │
│   - Tags & categories   │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│   Scheduler & Publisher  │
│   - Calendar view       │
│   - Queue management    │
│   - Auto-publish        │
│   - Platform adaptation │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│   Analytics & Funnel    │
│   - Engagement metrics  │
│   - Funnel conversion   │
│   - A/B test results    │
│   - ROI tracking        │
└─────────────────────────┘
```

---

## GUI Mockups

### Main Navigation

```
┌─────────────────────────────────────────────────────────────────────────┐
│  PacketCode Marketing                                          ─ □ ✕  │
├─────────────────────────────────────────────────────────────────────────┤
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐              │
│  │Studio│ │Library│ │ Post │ │Funnel│ │Email │ │Stats │              │
│  └──────┘ └──────┘ └──────┘ └──────┘ └──────┘ └──────┘              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│                         [Active View Area]                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 1. Content Studio View

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Content Studio                                       [+ New Content]   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─ Source Material ─────────────────────────────────────────────────┐  │
│  │                                                                   │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │  │
│  │  │ 📹 Video │  │ 📝 Text  │  │ 📋 Change│  │ 🔗 URL   │        │  │
│  │  │ Upload   │  │ Paste    │  │   log     │  │ Import   │        │  │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │  │
│  │                                                                   │  │
│  │  Drop a screen recording, paste text, or import from URL          │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌─ Generation Pipeline ────────────────────────────────────────────┐  │
│  │                                                                   │  │
│  │  Source: coding-session-2026-03-01.mp4 (47:23)                    │  │
│  │                                                                   │  │
│  │  Generate:                                                        │  │
│  │  ┌─────────────────────────────────────────────────────────────┐  │  │
│  │  │ [x] Short clips (TikTok/Reels)    Engine: Opus Clip        │  │  │
│  │  │ [x] Social media posts            Engine: Claude API       │  │  │
│  │  │ [ ] AI avatar video               Engine: HeyGen           │  │  │
│  │  │ [ ] Blog post                     Engine: Claude API       │  │  │
│  │  │ [x] Twitter/X thread              Engine: Claude API       │  │  │
│  │  │ [ ] Reddit post                   Engine: Claude API       │  │  │
│  │  └─────────────────────────────────────────────────────────────┘  │  │
│  │                                                                   │  │
│  │  Tone: [Technical ▾]   Brand voice: [PacketCode defaults ▾]      │  │
│  │                                                                   │  │
│  │                              [🚀 Generate All]                    │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌─ Review Queue (7 items pending) ─────────────────────────────────┐  │
│  │                                                                   │  │
│  │  ┌─────┬───────────────────────────────┬──────────┬───────────┐  │  │
│  │  │ Type│ Preview                       │ Platform │ Actions   │  │  │
│  │  ├─────┼───────────────────────────────┼──────────┼───────────┤  │  │
│  │  │ 📹  │ "Multi-agent magic in 30s..." │ TikTok   │ ✓ ✏ ✕    │  │  │
│  │  │ 📹  │ "Deploy pipeline demo..."     │ Reels    │ ✓ ✏ ✕    │  │  │
│  │  │ 📝  │ "Just shipped MCP Hub in..."  │ X/Twitter│ ✓ ✏ ✕    │  │  │
│  │  │ 📝  │ "PacketCode v0.4: What's..."  │ Reddit   │ ✓ ✏ ✕    │  │  │
│  │  │ 📹  │ "Why we built PacketCode..."  │ YouTube  │ ✓ ✏ ✕    │  │  │
│  │  │ 📝  │ "Thread: Building an AI..."   │ X/Twitter│ ✓ ✏ ✕    │  │  │
│  │  │ 📝  │ "PacketCode wraps Claude..."  │ LinkedIn │ ✓ ✏ ✕    │  │  │
│  │  └─────┴───────────────────────────────┴──────────┴───────────┘  │  │
│  │                                                                   │  │
│  │  [✓ Approve All]  [Send to Library]  [Regenerate Selected]       │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 2. Content Library View

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Content Library                      🔍 Search    [Filter ▾] [Sort ▾] │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Tabs: [ All (47) | Video (12) | Text (28) | Image (7) | Scheduled (9)]│
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │                                                                     ││
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐   ││
│  │  │ 📹         │  │ 📹         │  │ 📝         │  │ 📝         │   ││
│  │  │ [thumb]    │  │ [thumb]    │  │ Thread:    │  │ Post:      │   ││
│  │  │            │  │            │  │ Building   │  │ Just       │   ││
│  │  │ Multi-agent│  │ Deploy     │  │ an AI IDE  │  │ shipped    │   ││
│  │  │ demo 30s   │  │ pipeline   │  │ with Tauri │  │ MCP Hub... │   ││
│  │  │            │  │ demo 45s   │  │            │  │            │   ││
│  │  │ TikTok     │  │ Reels      │  │ X/Twitter  │  │ Reddit     │   ││
│  │  │ ● Ready    │  │ ● Ready    │  │ ◐ Draft    │  │ ● Ready    │   ││
│  │  │            │  │            │  │            │  │            │   ││
│  │  │ [Schedule] │  │ [Schedule] │  │ [Edit]     │  │ [Schedule] │   ││
│  │  └────────────┘  └────────────┘  └────────────┘  └────────────┘   ││
│  │                                                                     ││
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐   ││
│  │  │ 📹         │  │ 📝         │  │ 🖼️         │  │ 📝         │   ││
│  │  │ [thumb]    │  │ Post:      │  │ [thumb]    │  │ Post:      │   ││
│  │  │            │  │ PacketCode │  │            │  │ One        │   ││
│  │  │ Vibe       │  │ v0.4 is    │  │ Feature    │  │ cockpit.   │   ││
│  │  │ Architect  │  │ here...    │  │ comparison │  │ Every AI   │   ││
│  │  │ demo 60s   │  │            │  │ chart      │  │ agent...   │   ││
│  │  │ YouTube    │  │ LinkedIn   │  │ X/Twitter  │  │ Threads    │   ││
│  │  │ ✓ Posted   │  │ ◷ Sched.   │  │ ● Ready    │  │ ● Ready    │   ││
│  │  │            │  │ Mar 3 9am  │  │            │  │            │   ││
│  │  │ [Analytics]│  │ [Edit]     │  │ [Schedule] │  │ [Schedule] │   ││
│  │  └────────────┘  └────────────┘  └────────────┘  └────────────┘   ││
│  │                                                                     ││
│  └─────────────────────────────────────────────────────────────────────┘│
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 3. Schedule & Post View (Calendar)

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Schedule & Post                              ◀ March 2026 ▶  [+ Post] │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  View: [ Calendar | Queue | History ]    Platforms: [All ▾]             │
│                                                                         │
│  ┌──────┬──────┬──────┬──────┬──────┬──────┬──────┐                    │
│  │ Mon  │ Tue  │ Wed  │ Thu  │ Fri  │ Sat  │ Sun  │                    │
│  ├──────┼──────┼──────┼──────┼──────┼──────┼──────┤                    │
│  │  2   │  3   │  4   │  5   │  6   │  7   │  8   │                    │
│  │      │ 9:00 │      │10:00 │      │      │      │                    │
│  │      │🐦 X  │      │📸 IG │      │      │      │                    │
│  │      │"Just │      │Reel: │      │      │      │                    │
│  │      │ship..│      │Demo  │      │      │      │                    │
│  │      │      │      │      │      │      │      │                    │
│  │      │14:00 │      │14:00 │      │      │      │                    │
│  │      │🎵 TT │      │💼 LI │      │      │      │                    │
│  │      │Clip: │      │"Pack │      │      │      │                    │
│  │      │Multi │      │etCod │      │      │      │                    │
│  │      │agent │      │e v0. │      │      │      │                    │
│  ├──────┼──────┼──────┼──────┼──────┼──────┼──────┤                    │
│  │  9   │ 10   │ 11   │ 12   │ 13   │ 14   │ 15   │                    │
│  │      │ 9:00 │      │ 9:00 │      │      │      │                    │
│  │      │👽 RD │      │🐦 X  │      │      │      │                    │
│  │      │Show  │      │Threa │      │      │      │                    │
│  │      │HN ...|      │d: Bu │      │      │      │                    │
│  │      │      │      │ildin │      │      │      │                    │
│  │      │14:00 │      │      │      │      │      │                    │
│  │      │📺 YT │      │      │      │      │      │                    │
│  │      │Deep  │      │      │      │      │      │                    │
│  │      │dive  │      │      │      │      │      │                    │
│  └──────┴──────┴──────┴──────┴──────┴──────┴──────┘                    │
│                                                                         │
│  ┌─ Upcoming Queue ──────────────────────────────────────────────────┐  │
│  │ Mar 3  09:00  🐦  "Just shipped MCP Hub in PacketCode..."  [Edit] │  │
│  │ Mar 3  14:00  🎵  Clip: Multi-agent orchestration (30s)    [Edit] │  │
│  │ Mar 5  10:00  📸  Reel: Deploy pipeline demo (45s)         [Edit] │  │
│  │ Mar 5  14:00  💼  "PacketCode v0.4 is here..."             [Edit] │  │
│  │ Mar 10 09:00  👽  Show HN: PacketCode — Multi-agent IDE    [Edit] │  │
│  │ Mar 10 14:00  📺  Deep dive: Building an AI IDE (28:14)    [Edit] │  │
│  │ Mar 12 09:00  🐦  Thread: Building an AI IDE with Tauri    [Edit] │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  Platform legend: 🐦 X/Twitter  🎵 TikTok  📸 Instagram               │
│  📺 YouTube  💼 LinkedIn  👽 Reddit  🧵 Threads  📘 Facebook           │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 4. Funnel Builder View (React Flow Node Editor)

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Funnel Builder                    [Save] [Preview] [Analytics Overlay] │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Funnels: [ Main Launch ▾ ]  [+ New Funnel]  [Duplicate]  [Export JSON] │
│                                                                         │
│  ┌─ Node Palette ─┐  ┌─ Canvas ─────────────────────────────────────┐  │
│  │                 │  │                                               │  │
│  │ ── Sources ──   │  │                                               │  │
│  │ [Social Post ]  │  │   ┌─────────────┐                            │  │
│  │ [Blog Post   ]  │  │   │ 🌐 AWARENESS│                            │  │
│  │ [YouTube Vid ]  │  │   │             │                            │  │
│  │ [HN Launch   ]  │  │   │ HN Launch   │                            │  │
│  │ [PH Launch   ]  │  │   │ TikTok Clips│                            │  │
│  │                 │  │   │ X Threads   │                            │  │
│  │ ── Pages ────   │  │   │ Reddit Post │                            │  │
│  │ [Landing Page]  │  │   │             │        ┌─────────────┐     │  │
│  │ [Docs Page   ]  │  │   │ Est: 50K    │───────▶│ 🔍 EXPLORE  │     │  │
│  │ [Pricing Page]  │  │   │ impressions │        │             │     │  │
│  │                 │  │   └─────────────┘        │ GitHub Repo │     │  │
│  │ ── Actions ──   │  │                          │ README      │     │  │
│  │ [Email Seq.  ]  │  │                          │ Landing Pg  │     │  │
│  │ [Discord Inv.]  │  │                          │ Docs        │     │  │
│  │ [GitHub Star ]  │  │                          │             │     │  │
│  │ [Download    ]  │  │   ┌─────────────┐        │ Est: 5K     │     │  │
│  │ [Upgrade     ]  │  │   │ 📧 NURTURE  │◀───────│ visitors    │     │  │
│  │                 │  │   │             │        └─────────────┘     │  │
│  │ ── Metrics ──   │  │   │ Welcome     │                            │  │
│  │ [Conv. Rate  ]  │  │   │ Email Seq   │                            │  │
│  │ [Drop-off    ]  │  │   │ Discord Inv │        ┌─────────────┐     │  │
│  │ [Engagement  ]  │  │   │ Weekly      │───────▶│ ⬇ ACTIVATE  │     │  │
│  │                 │  │   │ Changelog   │        │             │     │  │
│  │                 │  │   │             │        │ Download    │     │  │
│  │                 │  │   │ Est: 2K     │        │ First Run   │     │  │
│  │                 │  │   │ subscribers │        │ First Sess. │     │  │
│  │                 │  │   └─────────────┘        │ Return Day7 │     │  │
│  │                 │  │                          │             │     │  │
│  │                 │  │                          │ Est: 500    │     │  │
│  │                 │  │                          │ active users│     │  │
│  │                 │  │                          └──────┬──────┘     │  │
│  │                 │  │                                 │            │  │
│  │                 │  │   ┌─────────────┐               │            │  │
│  │                 │  │   │ 💰 CONVERT  │◀──────────────┘            │  │
│  │                 │  │   │             │                            │  │
│  │                 │  │   │ Pro Upgrade │        ┌─────────────┐     │  │
│  │                 │  │   │ Teams Tier  │───────▶│ 📣 ADVOCATE │     │  │
│  │                 │  │   │             │        │             │     │  │
│  │                 │  │   │ Target: 3%  │        │ Contributor │     │  │
│  │                 │  │   │ = 15 users  │        │ Ambassador  │     │  │
│  │                 │  │   │ = $180/mo   │        │ Case Study  │     │  │
│  │                 │  │   └─────────────┘        │ Referral    │     │  │
│  │                 │  │                          └─────────────┘     │  │
│  │                 │  │                                               │  │
│  └─────────────────┘  └─────────────────────────────────────────────┘  │
│                                                                         │
│  ┌─ Stage Properties ───────────────────────────────────────────────┐  │
│  │ Selected: ACTIVATE                                                │  │
│  │ Events tracked: download, first_launch, first_session, return_d7  │  │
│  │ PostHog funnel ID: funnel_activate_001                            │  │
│  │ Conversion target: 10% of explorers                               │  │
│  │ Email trigger: "first_launch" → Welcome sequence                  │  │
│  │ Actions: [Edit Events] [Set Target] [Configure Email] [View Data] │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 5. Email Sequences View

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Email Sequences                                    [+ New Sequence]    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─ Active Sequences ───────────────────────────────────────────────┐  │
│  │                                                                   │  │
│  │  ┌─ Welcome Onboarding (triggered by: first_launch) ──────────┐  │  │
│  │  │                                                             │  │  │
│  │  │  Day 0          Day 3           Day 7          Day 14       │  │  │
│  │  │  ┌────────┐    ┌────────┐    ┌────────┐    ┌────────┐      │  │  │
│  │  │  │Welcome │───▶│Did you │───▶│Case    │───▶│Monthly │      │  │  │
│  │  │  │+ Quick │    │try MCP │    │study:  │    │summary │      │  │  │
│  │  │  │start   │    │Hub?    │    │Solo dev│    │+ tips  │      │  │  │
│  │  │  │        │    │        │    │story   │    │        │      │  │  │
│  │  │  │Open:47%│    │Open:32%│    │Open:28%│    │Open:25%│      │  │  │
│  │  │  │Click:12│    │Click:8%│    │Click:6%│    │Click:5%│      │  │  │
│  │  │  └────────┘    └────────┘    └────────┘    └────────┘      │  │  │
│  │  │                                                             │  │  │
│  │  │  Subscribers: 347    Active: 289    Completed: 58           │  │  │
│  │  │  [Edit] [Pause] [Analytics] [A/B Test]                     │  │  │
│  │  └─────────────────────────────────────────────────────────────┘  │  │
│  │                                                                   │  │
│  │  ┌─ Upgrade Nudge (triggered by: usage_limit_80%) ────────────┐  │  │
│  │  │  Day 0: "You're using 80% of free sessions..."             │  │  │
│  │  │  Day 3: "Unlock unlimited sessions for $12/mo"             │  │  │
│  │  │  Subscribers: 42   Conversion: 7.1%                        │  │  │
│  │  │  [Edit] [Pause] [Analytics]                                │  │  │
│  │  └─────────────────────────────────────────────────────────────┘  │  │
│  │                                                                   │  │
│  │  ┌─ Weekly Changelog (triggered by: every Monday 9am) ────────┐  │  │
│  │  │  Auto-generated from GitHub releases + manual additions     │  │  │
│  │  │  Subscribers: 1,247   Avg open rate: 34%                   │  │  │
│  │  │  [Edit Template] [Preview Next] [Analytics]                │  │  │
│  │  └─────────────────────────────────────────────────────────────┘  │  │
│  │                                                                   │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌─ Email Editor ────────────────────────────────────────────────────┐  │
│  │  Template: Welcome Email                                          │  │
│  │  Subject: Welcome to PacketCode — here's your quickstart          │  │
│  │  ┌────────────────────────────────────────────────────────────┐   │  │
│  │  │ # Welcome to PacketCode!                                   │   │  │
│  │  │                                                            │   │  │
│  │  │ You just joined {{subscriber_count}} developers using      │   │  │
│  │  │ PacketCode to orchestrate AI agents.                       │   │  │
│  │  │                                                            │   │  │
│  │  │ ## Get started in 2 minutes:                               │   │  │
│  │  │                                                            │   │  │
│  │  │ ```bash                                                    │   │  │
│  │  │ # Launch PacketCode and create your first session          │   │  │
│  │  │ # Choose Claude Code or Codex CLI                          │   │  │
│  │  │ # Start coding with AI                                     │   │  │
│  │  │ ```                                                        │   │  │
│  │  │                                                            │   │  │
│  │  │ [Read the docs →](https://packetcode.dev/docs)             │   │  │
│  │  │ [Join Discord →](https://discord.gg/packetcode)            │   │  │
│  │  └────────────────────────────────────────────────────────────┘   │  │
│  │  Format: Markdown (auto-converts to HTML)   [Preview] [Send Test]│  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 6. Analytics Dashboard View

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Analytics Dashboard                  Period: [Last 30 days ▾] [Export] │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐│
│  │ Impressns│  │  Clicks  │  │Downloads │  │ Active   │  │  Paid    ││
│  │  127,450 │  │   4,891  │  │    847   │  │   312    │  │    14    ││
│  │ +23% ▲   │  │ +15% ▲   │  │ +31% ▲   │  │ +18% ▲   │  │ +40% ▲  ││
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘  └──────────┘│
│                                                                         │
│  ┌─ Funnel Conversion ──────────────────────────────────────────────┐  │
│  │                                                                   │  │
│  │  Awareness    Explore     Activate     Convert     Advocate       │  │
│  │  ████████████████████████████████████████████████████████████████  │  │
│  │  127,450      4,891        847          312          14           │  │
│  │       │          │           │            │           │            │  │
│  │      3.8%       17.3%       36.8%        4.5%                     │  │
│  │                                                                   │  │
│  │  ▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔  │  │
│  │  ██████████████████████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  │  │
│  │  ███████████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  │  │
│  │  █████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  │  │
│  │  ██░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  │  │
│  │                                                                   │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌─ Platform Performance ──────────┐  ┌─ Top Content ──────────────┐  │
│  │                                  │  │                            │  │
│  │  Platform     Posts  Eng.  Conv. │  │  1. "Multi-agent demo"     │  │
│  │  ─────────────────────────────── │  │     TikTok · 47K views     │  │
│  │  🐦 X/Twitter   28   4.2%  2.1% │  │     3.2K likes · 891 clicks│  │
│  │  🎵 TikTok      15   6.8%  1.4% │  │                            │  │
│  │  📺 YouTube      8   3.1%  4.7% │  │  2. "Show HN: PacketCode"  │  │
│  │  👽 Reddit       6   2.9%  5.2% │  │     HN · 342 points        │  │
│  │  💼 LinkedIn    12   1.8%  3.3% │  │     127 comments · 423 cl. │  │
│  │  📸 Instagram   10   5.1%  0.8% │  │                            │  │
│  │  📘 Facebook     4   1.2%  0.6% │  │  3. "Building AI IDE"      │  │
│  │  🧵 Threads      7   2.4%  0.9% │  │     YouTube · 12K views    │  │
│  │                                  │  │     847 likes · 234 clicks │  │
│  └──────────────────────────────────┘  └────────────────────────────┘  │
│                                                                         │
│  ┌─ Revenue Tracking ───────────────────────────────────────────────┐  │
│  │                                                                   │  │
│  │  MRR: $168     Subscribers: 14     Churn: 0%     LTV est: $144   │  │
│  │                                                                   │  │
│  │  ┌──────────────────────────────────────────────────────────────┐ │  │
│  │  │     $200 ┤                                              ●    │ │  │
│  │  │          │                                         ●         │ │  │
│  │  │     $150 ┤                                    ●              │ │  │
│  │  │          │                               ●                   │ │  │
│  │  │     $100 ┤                          ●                        │ │  │
│  │  │          │                     ●                             │ │  │
│  │  │      $50 ┤               ●                                   │ │  │
│  │  │          │          ●                                        │ │  │
│  │  │       $0 ┤─────●───────────────────────────────────────────  │ │  │
│  │  │          Jan   Feb   Mar   Apr   May   Jun   Jul   Aug       │ │  │
│  │  └──────────────────────────────────────────────────────────────┘ │  │
│  │                                                                   │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Module Specifications

### Module 1: Content Studio

**Purpose:** Transform raw material into platform-ready content using AI.

**Features:**
- Video upload → auto-clip into 30-60 second shorts (via Opus Clip API)
- Text input → platform-specific social posts (via Claude/OpenAI API)
- Changelog → newsletter + social posts (via AI + templates)
- URL import → summarized social posts
- AI avatar video generation (via HeyGen API)
- Text-to-video promo generation (via Zebracat API)
- Human review queue with approve/edit/reject workflow
- Brand voice configuration (tone, vocabulary, emoji rules)
- Platform-specific formatting (hashtags, character limits, aspect ratios)

**AI Prompt Templates (stored as JSON configs):**

```json
{
  "templates": {
    "twitter_post": {
      "system": "You are a developer marketing copywriter. Write concise, technical, authentic social media posts. No marketing jargon. No emojis unless specified. Include a clear value proposition.",
      "user": "Write a Twitter/X post (max 280 chars) about this PacketCode update:\n\n{{content}}\n\nTone: {{tone}}\nInclude link: {{link}}"
    },
    "reddit_post": {
      "system": "You are a developer sharing a genuine project update on Reddit. Be humble, technical, and honest about limitations. Reddit hates marketing speak.",
      "user": "Write a Reddit post for r/{{subreddit}} about:\n\n{{content}}\n\nFormat: Title + body. Be genuine and invite feedback."
    },
    "tiktok_script": {
      "system": "You are writing a script for a 30-60 second TikTok video about a coding tool. Be energetic but authentic. Hook in first 3 seconds.",
      "user": "Write a TikTok script showcasing:\n\n{{feature}}\n\nHook: grab attention immediately\nDemo: show the feature in action\nCTA: link in bio"
    },
    "linkedin_post": {
      "system": "You are a technical founder sharing professional updates on LinkedIn. Professional tone, specific metrics where possible, show technical depth.",
      "user": "Write a LinkedIn post about:\n\n{{content}}\n\nAudience: technical leaders and developers"
    }
  }
}
```

### Module 2: Content Library

**Purpose:** Organize, tag, and manage all generated content.

**Features:**
- Grid/list view of all content assets
- Status tracking: Draft → Ready → Scheduled → Posted → Archived
- Platform tags (multi-platform content linked together)
- Campaign grouping
- Search and filter by type, platform, status, date, campaign
- Duplicate and adapt content for new platforms
- Version history

### Module 3: Scheduler & Publisher

**Purpose:** Schedule and auto-publish content across platforms.

**Features:**
- Visual calendar (week/month view)
- Drag-and-drop scheduling
- Optimal posting time suggestions per platform
- Queue management with priority ordering
- Auto-publish via Postiz API or Late.dev unified API
- Retry logic for failed posts
- Platform-specific preview (character limits, image crops, video formats)
- Recurring post templates (weekly changelog, daily tip)
- Time zone handling

**Posting Schedule Defaults:**

```json
{
  "defaults": {
    "twitter": { "times": ["09:00", "14:00"], "days": ["mon", "wed", "fri"] },
    "tiktok": { "times": ["14:00", "19:00"], "days": ["tue", "thu", "sat"] },
    "reddit": { "times": ["09:00"], "days": ["mon"], "note": "max 1/week, manual review required" },
    "linkedin": { "times": ["10:00"], "days": ["tue", "thu"] },
    "youtube": { "times": ["10:00"], "days": ["wed"] },
    "instagram": { "times": ["12:00", "18:00"], "days": ["mon", "wed", "fri"] }
  },
  "timezone": "America/Chicago"
}
```

### Module 4: Funnel Builder

**Purpose:** Visually design and track marketing funnels.

**Features:**
- React Flow-based node/edge editor
- Pre-built stage templates (Awareness, Explore, Activate, Convert, Advocate)
- Custom stage creation
- Event binding (connect stages to PostHog events)
- Conversion target setting per stage
- Email sequence triggers per stage
- Real-time analytics overlay (conversion rates, drop-off points)
- Multiple funnel support (launch funnel, content funnel, upgrade funnel)
- Export/import as JSON
- Funnel comparison (A/B test different funnels)

**Stage Types:**

```typescript
type FunnelStageType =
  | 'awareness'    // Top-of-funnel: social posts, HN, PH, ads
  | 'explore'      // Discovery: GitHub repo, landing page, docs
  | 'nurture'      // Engagement: email sequences, Discord, newsletters
  | 'activate'     // First use: download, first launch, first session
  | 'convert'      // Payment: upgrade to Pro/Teams
  | 'advocate'     // Expansion: contributor, ambassador, referral
  | 'custom';      // User-defined stage
```

### Module 5: Email Engine

**Purpose:** Event-driven email automation.

**Features:**
- Markdown-based email editor (auto-converts to HTML via React Email)
- Event-triggered sequences (tied to PostHog events)
- Time-delayed sequences (Day 0, Day 3, Day 7, Day 14)
- A/B testing for subject lines and content
- Subscriber management (lists, segments, tags)
- Analytics: open rate, click rate, unsubscribe rate
- Templates: welcome, changelog, upgrade nudge, milestone celebration
- Variables: `{{name}}`, `{{subscriber_count}}`, `{{days_active}}`
- Integration: Resend API (primary) or Loops API (alternative)

### Module 6: Analytics Dashboard

**Purpose:** Unified view of all marketing metrics.

**Features:**
- KPI cards: impressions, clicks, downloads, active users, paid users, MRR
- Funnel visualization with conversion rates
- Platform performance comparison
- Top content ranking by engagement/conversion
- Revenue tracking (MRR, churn, LTV)
- Time-series charts (daily/weekly/monthly)
- Attribution: which content/platform drove each conversion
- Export to CSV/JSON
- PostHog integration for product analytics
- Social media analytics via platform APIs

---

## Content Generation Pipeline

### Detailed Pipeline: Screen Recording → Social Content

```
Step 1: RECORD
├── Tool: OBS Studio (free) or FocuSee ($)
├── Output: .mp4 file (raw coding session, 15-60 minutes)
└── Storage: /content/raw/

Step 2: AUTO-CLIP
├── Tool: Opus Clip API
├── Input: Raw .mp4
├── Process: AI identifies "highlight" moments, assigns viral scores
├── Output: 5-20 short clips (30-60 seconds each) with captions
├── Format: 9:16 (TikTok/Reels/Shorts) + 16:9 (YouTube/LinkedIn)
└── Storage: /content/clips/

Step 3: POLISH
├── Tool: CapCut (manual) or API-based caption overlay
├── Process: Add branded intro/outro, enhance captions, adjust timing
├── Output: Platform-ready video files
└── Storage: /content/ready/

Step 4: GENERATE TEXT
├── Tool: Claude API / OpenAI API
├── Input: Clip description + brand voice config
├── Process: Generate platform-specific captions, posts, threads
├── Output: JSON with text content per platform
└── Storage: /content/text/

Step 5: HUMAN REVIEW
├── Interface: Content Studio review queue
├── Process: Approve, edit, or reject each piece
├── Output: Approved content moved to Content Library
└── Status: Ready for scheduling

Step 6: SCHEDULE
├── Interface: Calendar view
├── Process: Drag content to time slots or auto-schedule
├── Output: Queued posts with timestamps
└── Integration: Postiz API / Late.dev

Step 7: PUBLISH
├── Tool: Postiz (self-hosted) or Late.dev unified API
├── Process: Auto-publish at scheduled time
├── Platforms: TikTok, Instagram, YouTube, X, LinkedIn, Reddit, Threads, Facebook
├── Retry: 3 attempts with exponential backoff
└── Status: Posted → tracked in analytics

Step 8: TRACK
├── Tool: PostHog + platform native analytics
├── Metrics: Views, engagement, clicks, conversions
├── Attribution: Link each conversion to source content/platform
└── Feedback: Inform future content generation (what performs best)
```

### Pipeline: Changelog → Multi-Platform Content

```
Step 1: EXTRACT
├── Source: GitHub Releases API or CHANGELOG.md
├── Process: Parse version, features, fixes, breaking changes
└── Output: Structured changelog data

Step 2: GENERATE
├── Tool: Claude API with templates
├── Generate:
│   ├── Newsletter email (Markdown → React Email)
│   ├── Twitter/X thread (feature highlights)
│   ├── LinkedIn post (professional summary)
│   ├── Reddit post (honest, technical)
│   ├── TikTok script (if visual features)
│   └── Discord announcement
└── Output: Platform-specific content in review queue

Step 3: REVIEW → SCHEDULE → PUBLISH → TRACK
├── Same as Steps 5-8 above
└── Automated weekly cadence (every Monday)
```

---

## Social Media Distribution Engine

### API Integration Layer

#### Option A: Postiz (Self-Hosted, Recommended)

```
Postiz REST API (self-hosted at postiz.local or cloud)
├── POST /public/v1/posts          — Create and schedule post
├── GET  /public/v1/posts          — List all posts
├── GET  /public/v1/posts/:id      — Get post details
├── PUT  /public/v1/posts/:id      — Update post
├── DELETE /public/v1/posts/:id    — Delete post
├── GET  /public/v1/analytics      — Get analytics data
└── Auth: API key in X-API-KEY header
    Rate limit: 30 req/hour (cloud), unlimited (self-hosted)

Supported platforms: Facebook, Instagram, TikTok, YouTube, Reddit,
LinkedIn, Dribbble, Threads, Pinterest, X, Bluesky, Mastodon, Discord
```

#### Option B: Late.dev (Unified API, Alternative)

```
Late.dev REST API
├── Single API covering 13 platforms
├── Handles OAuth, rate limiting, format normalization
├── SDKs: Node.js, Python, Go, Java, PHP, .NET, Rust
└── Abstracts platform differences behind consistent interface
```

### Platform-Specific Adapters

```typescript
interface PlatformAdapter {
  platform: Platform;
  maxTextLength: number;
  supportedMedia: MediaType[];
  aspectRatios: string[];
  hashtagSupport: boolean;
  linkSupport: boolean;

  formatContent(content: Content): PlatformContent;
  validate(content: PlatformContent): ValidationResult;
  publish(content: PlatformContent): Promise<PublishResult>;
  getAnalytics(postId: string): Promise<AnalyticsData>;
}

// Platform configs
const PLATFORM_CONFIGS = {
  twitter:   { maxText: 280,   media: ['image', 'video', 'gif'], ratio: '16:9', hashtags: true },
  tiktok:    { maxText: 2200,  media: ['video'],                 ratio: '9:16', hashtags: true },
  instagram: { maxText: 2200,  media: ['image', 'video', 'carousel'], ratio: '1:1,9:16', hashtags: true },
  reddit:    { maxText: 40000, media: ['image', 'video', 'link'], ratio: 'any', hashtags: false },
  linkedin:  { maxText: 3000,  media: ['image', 'video', 'document'], ratio: '1:1,16:9', hashtags: true },
  youtube:   { maxText: 5000,  media: ['video'],                 ratio: '16:9', hashtags: true },
  facebook:  { maxText: 63206, media: ['image', 'video', 'link'], ratio: 'any', hashtags: true },
  threads:   { maxText: 500,   media: ['image', 'video'],        ratio: '1:1,9:16', hashtags: true },
};
```

---

## Funnel Builder & Manager

### Funnel Data Model

```typescript
interface Funnel {
  id: string;
  name: string;
  description: string;
  stages: FunnelStage[];
  edges: FunnelEdge[];
  createdAt: string;
  updatedAt: string;
  isActive: boolean;
}

interface FunnelStage {
  id: string;
  type: FunnelStageType;
  name: string;
  description: string;
  position: { x: number; y: number };  // React Flow position

  // Content sources for this stage
  contentSources: ContentSource[];

  // PostHog event tracking
  trackingEvents: string[];        // e.g., ['page_view', 'download', 'first_launch']
  posthogFunnelId?: string;        // Link to PostHog funnel insight

  // Targets
  conversionTarget: number;        // Target conversion rate (%)
  estimatedVolume: number;         // Expected traffic/users

  // Email triggers
  emailTriggers: EmailTrigger[];

  // Measured metrics (populated from PostHog)
  metrics?: {
    totalUsers: number;
    convertedUsers: number;
    conversionRate: number;
    avgTimeInStage: number;        // seconds
  };
}

interface FunnelEdge {
  id: string;
  source: string;                  // Stage ID
  target: string;                  // Stage ID
  label?: string;                  // e.g., "3.8% convert"
  conversionRate?: number;         // Measured conversion
}

interface ContentSource {
  type: 'social_post' | 'blog' | 'video' | 'landing_page' | 'docs' | 'email' | 'discord';
  platform?: Platform;
  url?: string;
  contentId?: string;              // Link to Content Library item
}

interface EmailTrigger {
  event: string;                   // PostHog event name
  sequenceId: string;              // Email sequence to trigger
  delay?: number;                  // Delay in seconds after event
}
```

### Pre-Built Funnel Templates

#### Template: "Open Source Launch Funnel"

```json
{
  "name": "Open Source Launch Funnel",
  "stages": [
    {
      "type": "awareness",
      "name": "Awareness",
      "description": "Get the word out about PacketCode",
      "contentSources": [
        { "type": "social_post", "platform": "twitter" },
        { "type": "social_post", "platform": "tiktok" },
        { "type": "social_post", "platform": "reddit" },
        { "type": "social_post", "platform": "linkedin" },
        { "type": "video", "platform": "youtube" }
      ],
      "trackingEvents": ["utm_landing_page_view"],
      "conversionTarget": 5,
      "estimatedVolume": 50000
    },
    {
      "type": "explore",
      "name": "Explore",
      "description": "Visitors check out the repo, docs, and landing page",
      "contentSources": [
        { "type": "landing_page", "url": "https://packetcode.dev" },
        { "type": "docs", "url": "https://packetcode.dev/docs" },
        { "type": "social_post", "platform": "github" }
      ],
      "trackingEvents": ["github_repo_view", "docs_page_view", "landing_page_view"],
      "conversionTarget": 15,
      "estimatedVolume": 2500
    },
    {
      "type": "activate",
      "name": "Activate",
      "description": "User downloads, installs, and creates first session",
      "trackingEvents": ["download", "first_launch", "first_session_created"],
      "conversionTarget": 40,
      "estimatedVolume": 375,
      "emailTriggers": [
        { "event": "first_launch", "sequenceId": "welcome_onboarding", "delay": 0 }
      ]
    },
    {
      "type": "nurture",
      "name": "Retain",
      "description": "User returns within 7 days and becomes regular",
      "contentSources": [
        { "type": "email", "platform": "resend" },
        { "type": "discord" }
      ],
      "trackingEvents": ["return_day_7", "session_created_count_5"],
      "conversionTarget": 50,
      "estimatedVolume": 150,
      "emailTriggers": [
        { "event": "return_day_7", "sequenceId": "power_user_tips", "delay": 86400 }
      ]
    },
    {
      "type": "convert",
      "name": "Convert",
      "description": "User upgrades to Pro tier",
      "trackingEvents": ["upgrade_initiated", "payment_completed"],
      "conversionTarget": 5,
      "estimatedVolume": 8,
      "emailTriggers": [
        { "event": "session_limit_reached", "sequenceId": "upgrade_nudge", "delay": 0 }
      ]
    },
    {
      "type": "advocate",
      "name": "Advocate",
      "description": "User becomes contributor, ambassador, or referrer",
      "trackingEvents": ["github_pr_created", "referral_link_shared", "community_post"],
      "conversionTarget": 20,
      "estimatedVolume": 2
    }
  ]
}
```

#### Template: "Content Marketing Funnel"

```json
{
  "name": "Content Marketing Funnel",
  "stages": [
    {
      "type": "awareness",
      "name": "Content Distribution",
      "contentSources": [
        { "type": "video", "platform": "tiktok" },
        { "type": "video", "platform": "youtube" },
        { "type": "social_post", "platform": "twitter" },
        { "type": "blog", "platform": "devto" }
      ],
      "trackingEvents": ["content_view", "social_click"]
    },
    {
      "type": "explore",
      "name": "Website Visit",
      "trackingEvents": ["landing_page_view", "docs_page_view", "pricing_page_view"]
    },
    {
      "type": "nurture",
      "name": "Email Subscriber",
      "trackingEvents": ["email_subscribed", "newsletter_opened"],
      "emailTriggers": [
        { "event": "email_subscribed", "sequenceId": "welcome_onboarding" }
      ]
    },
    {
      "type": "activate",
      "name": "Trial User",
      "trackingEvents": ["download", "first_launch"]
    },
    {
      "type": "convert",
      "name": "Paid User",
      "trackingEvents": ["upgrade_completed"]
    }
  ]
}
```

---

## Analytics Dashboard

### PostHog Integration

```typescript
interface PostHogConfig {
  host: string;        // Self-hosted URL or https://app.posthog.com
  apiKey: string;      // Project API key
  personalApiKey: string;  // For querying insights API
}

// Key events to track in PacketCode
const TRACKED_EVENTS = {
  // Website / landing
  'landing_page_view': { properties: ['utm_source', 'utm_medium', 'utm_campaign'] },
  'docs_page_view': { properties: ['page_path'] },
  'pricing_page_view': {},

  // Product
  'download': { properties: ['platform', 'version'] },
  'first_launch': { properties: ['platform', 'version'] },
  'first_session_created': { properties: ['agent_type'] },  // claude or codex
  'session_created': { properties: ['agent_type', 'session_count'] },
  'feature_used': { properties: ['feature_name'] },  // mcp_hub, scaffold, deploy, etc.
  'return_day_7': {},
  'return_day_30': {},

  // Conversion
  'session_limit_reached': {},
  'upgrade_initiated': { properties: ['plan'] },
  'payment_completed': { properties: ['plan', 'amount'] },
  'payment_failed': { properties: ['error'] },

  // Community
  'discord_joined': {},
  'github_star': {},
  'github_pr_created': {},
  'referral_link_shared': {},

  // Email
  'email_subscribed': { properties: ['source'] },
  'email_opened': { properties: ['sequence', 'email_index'] },
  'email_clicked': { properties: ['sequence', 'email_index', 'link'] },
  'email_unsubscribed': { properties: ['sequence'] },
};
```

### Dashboard Queries

```typescript
// Funnel query to PostHog
const funnelQuery = {
  insight: 'FUNNELS',
  events: [
    { id: 'landing_page_view', name: 'Landing Page View' },
    { id: 'download', name: 'Download' },
    { id: 'first_launch', name: 'First Launch' },
    { id: 'first_session_created', name: 'First Session' },
    { id: 'return_day_7', name: 'Return Day 7' },
    { id: 'payment_completed', name: 'Paid' },
  ],
  funnel_window_days: 30,
  date_from: '-30d',
  breakdown: 'utm_source',  // See which channels convert best
};

// Platform performance query
const platformQuery = {
  insight: 'TRENDS',
  events: [{ id: 'landing_page_view', math: 'total' }],
  breakdown: 'utm_source',
  date_from: '-30d',
  interval: 'week',
};
```

---

## Integration Layer

### External Service Connections

```typescript
interface IntegrationConfig {
  // Content Generation
  claudeApi: {
    apiKey: string;
    model: string;  // 'claude-sonnet-4-6' for content gen (cost-effective)
  };

  // Video Processing
  opusClip: {
    apiKey: string;
    webhookUrl: string;  // Callback when clips are ready
  };

  // AI Avatar Videos
  heyGen?: {
    apiKey: string;
    avatarId: string;
    voiceId: string;
  };

  // Social Media Distribution
  postiz: {
    baseUrl: string;     // Self-hosted URL
    apiKey: string;
  };
  // OR
  late?: {
    apiKey: string;
  };

  // Email
  resend: {
    apiKey: string;
    fromEmail: string;
    fromName: string;
  };

  // Analytics
  posthog: {
    host: string;
    projectApiKey: string;
    personalApiKey: string;
  };

  // GitHub (for changelog automation)
  github: {
    token: string;
    repo: string;  // 'owner/repo'
  };
}
```

### Webhook Architecture

```
Content Generation Complete
├── Opus Clip webhook → new clips available → add to review queue
├── HeyGen webhook → avatar video ready → add to review queue
└── AI text generation → immediate → add to review queue

Post Published
├── Postiz webhook → post published → update status in library
├── Track PostHog event: 'content_published'
└── Update analytics dashboard

Email Events
├── Resend webhook → email opened → track in PostHog
├── Resend webhook → email clicked → track in PostHog
└── Resend webhook → unsubscribed → update subscriber list

PostHog Events
├── Custom event fired → check email triggers → send email if matched
├── Funnel step completed → update funnel dashboard
└── Threshold reached → send notification (Discord webhook)
```

---

## Data Models

### Database Schema (SQLite for Desktop, PostgreSQL for Server)

```sql
-- Content items
CREATE TABLE content (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,              -- 'video', 'text', 'image'
  platform TEXT NOT NULL,          -- 'twitter', 'tiktok', etc.
  status TEXT NOT NULL DEFAULT 'draft',  -- 'draft', 'ready', 'scheduled', 'posted', 'archived'
  title TEXT,
  body TEXT,                       -- Text content or description
  media_path TEXT,                 -- Local file path for media
  media_url TEXT,                  -- Remote URL after upload
  campaign_id TEXT,
  source_material_id TEXT,         -- Link to original recording/text
  metadata TEXT,                   -- JSON: hashtags, mentions, etc.
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  posted_at TEXT,
  external_post_id TEXT            -- Platform's post ID
);

-- Scheduled posts
CREATE TABLE schedule (
  id TEXT PRIMARY KEY,
  content_id TEXT NOT NULL REFERENCES content(id),
  scheduled_at TEXT NOT NULL,
  timezone TEXT NOT NULL DEFAULT 'America/Chicago',
  status TEXT NOT NULL DEFAULT 'pending',  -- 'pending', 'publishing', 'published', 'failed'
  retry_count INTEGER DEFAULT 0,
  error_message TEXT,
  published_at TEXT,
  created_at TEXT NOT NULL
);

-- Funnels
CREATE TABLE funnels (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  stages TEXT NOT NULL,            -- JSON: FunnelStage[]
  edges TEXT NOT NULL,             -- JSON: FunnelEdge[]
  is_active BOOLEAN DEFAULT true,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

-- Email sequences
CREATE TABLE email_sequences (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  trigger_event TEXT NOT NULL,
  emails TEXT NOT NULL,            -- JSON: EmailStep[]
  is_active BOOLEAN DEFAULT true,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

-- Email subscribers
CREATE TABLE subscribers (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  name TEXT,
  tags TEXT,                       -- JSON: string[]
  source TEXT,                     -- Where they signed up
  subscribed_at TEXT NOT NULL,
  unsubscribed_at TEXT,
  metadata TEXT                    -- JSON: custom fields
);

-- Email send log
CREATE TABLE email_log (
  id TEXT PRIMARY KEY,
  subscriber_id TEXT NOT NULL REFERENCES subscribers(id),
  sequence_id TEXT NOT NULL REFERENCES email_sequences(id),
  email_index INTEGER NOT NULL,
  sent_at TEXT NOT NULL,
  opened_at TEXT,
  clicked_at TEXT,
  resend_id TEXT                   -- Resend's message ID
);

-- Analytics snapshots (cached from PostHog/platforms)
CREATE TABLE analytics_snapshots (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,              -- 'funnel', 'platform', 'content', 'revenue'
  data TEXT NOT NULL,              -- JSON
  period_start TEXT NOT NULL,
  period_end TEXT NOT NULL,
  captured_at TEXT NOT NULL
);

-- Campaigns
CREATE TABLE campaigns (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  funnel_id TEXT REFERENCES funnels(id),
  start_date TEXT,
  end_date TEXT,
  status TEXT NOT NULL DEFAULT 'draft',  -- 'draft', 'active', 'paused', 'completed'
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

### Zustand Store Structure

```typescript
// Main marketing store
interface MarketingStore {
  // Content Studio
  reviewQueue: ContentItem[];
  generationJobs: GenerationJob[];
  addToReviewQueue: (item: ContentItem) => void;
  approveContent: (id: string) => void;
  rejectContent: (id: string) => void;

  // Content Library
  library: ContentItem[];
  filters: ContentFilters;
  setFilters: (filters: ContentFilters) => void;
  searchLibrary: (query: string) => ContentItem[];

  // Scheduler
  scheduledPosts: ScheduledPost[];
  schedulePost: (contentId: string, scheduledAt: Date, platform: Platform) => void;
  cancelScheduled: (id: string) => void;

  // Funnel
  funnels: Funnel[];
  activeFunnel: string | null;
  createFunnel: (funnel: Funnel) => void;
  updateStage: (funnelId: string, stageId: string, updates: Partial<FunnelStage>) => void;
  setActiveFunnel: (id: string) => void;

  // Email
  sequences: EmailSequence[];
  subscribers: Subscriber[];
  createSequence: (seq: EmailSequence) => void;
  addSubscriber: (sub: Subscriber) => void;

  // Analytics
  dashboardData: DashboardData | null;
  refreshAnalytics: () => Promise<void>;

  // Settings
  integrations: IntegrationConfig;
  updateIntegration: (key: string, config: any) => void;
}
```

---

## Tech Stack

### Fits Existing PacketCode Stack

| Component | Technology | Why |
|---|---|---|
| **Frontend** | React 19 + TypeScript | Already in PacketCode |
| **State** | Zustand | Already in PacketCode |
| **Styling** | Tailwind CSS (dark theme tokens) | Already in PacketCode |
| **Icons** | lucide-react | Already in PacketCode |
| **Desktop wrapper** | Tauri v2 (Rust backend) | Already in PacketCode |
| **Visual funnel editor** | React Flow | MIT license, battle-tested, node-based graph editor |
| **Calendar** | @fullcalendar/react or custom | Scheduling view |
| **Charts** | recharts or @nivo/core | Analytics dashboard |
| **Markdown editor** | react-markdown + remark-gfm | Already in PacketCode |
| **Database** | SQLite (via Tauri SQL plugin) | Lightweight, desktop-native |
| **HTTP client** | reqwest (Rust) / fetch (TS) | Already in PacketCode |

### New Dependencies

```json
{
  "dependencies": {
    "@xyflow/react": "^12.0.0",
    "recharts": "^2.12.0",
    "@fullcalendar/react": "^6.1.0",
    "@fullcalendar/daygrid": "^6.1.0",
    "@dnd-kit/core": "^6.1.0",
    "date-fns": "^3.6.0"
  }
}
```

### Rust Backend Additions

```
src-tauri/src/commands/
  marketing/
    mod.rs           -- Module exports
    content.rs       -- Content CRUD, AI generation triggers
    scheduler.rs     -- Post scheduling, publishing
    funnel.rs        -- Funnel CRUD, analytics queries
    email.rs         -- Email sequence management, Resend integration
    analytics.rs     -- PostHog queries, dashboard data
    integrations.rs  -- External API connections (Postiz, Opus Clip, etc.)
```

---

## Implementation Plan

### Phase 1: Foundation (2-3 weeks)

| Task | Effort | Priority |
|---|---|---|
| Set up SQLite database with schema | 2 days | P0 |
| Create Zustand marketing store | 1 day | P0 |
| Build Content Library view (CRUD, grid, filters) | 3 days | P0 |
| Build basic Scheduler view (calendar, queue) | 3 days | P0 |
| Integrate Postiz API for publishing | 2 days | P0 |
| Add navigation and view routing | 1 day | P0 |

### Phase 2: Content Generation (2-3 weeks)

| Task | Effort | Priority |
|---|---|---|
| Build Content Studio UI (upload, generation options) | 3 days | P0 |
| Integrate Claude API for text generation | 2 days | P0 |
| Build review queue (approve/edit/reject) | 2 days | P0 |
| Integrate Opus Clip API for video clipping | 3 days | P1 |
| Add brand voice configuration | 1 day | P1 |
| Platform-specific content formatting | 2 days | P1 |

### Phase 3: Funnel Builder (2-3 weeks)

| Task | Effort | Priority |
|---|---|---|
| Integrate React Flow for visual editor | 3 days | P0 |
| Build stage node components | 2 days | P0 |
| Implement funnel templates | 1 day | P0 |
| Add stage properties panel | 2 days | P0 |
| Integrate PostHog for funnel analytics | 3 days | P1 |
| Build analytics overlay on funnel view | 2 days | P1 |

### Phase 4: Email & Analytics (2-3 weeks)

| Task | Effort | Priority |
|---|---|---|
| Build email sequence editor | 3 days | P1 |
| Integrate Resend API | 2 days | P1 |
| Build analytics dashboard | 3 days | P1 |
| Revenue tracking view | 2 days | P2 |
| Platform performance comparison | 2 days | P2 |
| Attribution tracking | 2 days | P2 |

### Phase 5: Polish & Automation (1-2 weeks)

| Task | Effort | Priority |
|---|---|---|
| Changelog → content automation | 2 days | P1 |
| Recurring post templates | 1 day | P2 |
| A/B testing for email | 2 days | P2 |
| Export/import funnels as JSON | 1 day | P2 |
| Keyboard shortcuts | 1 day | P2 |

**Total estimated effort: 8-12 weeks for full system**
**MVP (Library + Scheduler + Basic Studio): 4-5 weeks**

---

## Cost Analysis

### Monthly Operating Costs

| Service | Free Tier | Paid Tier | Notes |
|---|---|---|---|
| **Postiz** (self-hosted) | $0 | — | Unlimited on self-hosted |
| **PostHog** (self-hosted) | $0 | — | Unlimited on self-hosted |
| **Resend** | 100 emails/day free | $20/mo (50K emails) | Start free |
| **Claude API** (content gen) | — | ~$5-20/mo | Depends on volume |
| **Opus Clip** | 60 credits/mo free | $15/mo | 150 credits |
| **HeyGen** | 3 videos/mo free | $24/mo | If using avatars |
| **Zebracat** | Free tier | $25/mo | If using text-to-video |
| **Domain + hosting** | — | ~$10/mo | Landing page |

| Scenario | Monthly Cost |
|---|---|
| **Minimal (self-hosted everything, free tiers)** | $5-15/mo (API costs only) |
| **Standard (Opus Clip + Resend paid)** | $40-60/mo |
| **Full (all video tools + paid tiers)** | $80-120/mo |

### ROI Calculation

If the funnel system converts even 0.5% of social media impressions to downloads, and 3% of downloads to paid users:

| Monthly Impressions | Downloads (0.5%) | Paid Users (3%) | Revenue ($12/mo) |
|---|---|---|---|
| 10,000 | 50 | 2 | $24 |
| 50,000 | 250 | 8 | $96 |
| 100,000 | 500 | 15 | $180 |
| 500,000 | 2,500 | 75 | $900 |

Break-even at the "Standard" cost tier (~$50/mo) requires roughly 50,000 monthly impressions — achievable with consistent posting across 8 platforms.

---

## Summary

This funnel system is:
- **Feasible** with PacketCode's existing React/TypeScript/Tailwind/Tauri stack
- **MVP-able in 4-5 weeks** (content library + scheduler + basic AI generation)
- **Full system in 8-12 weeks** (including funnel builder, email, analytics)
- **Cheap to operate** ($5-120/month depending on tool usage)
- **Self-hosted and data-sovereign** (Postiz + PostHog + SQLite)
- **Extensible** — add new platforms, AI tools, and analytics sources via adapter pattern

The key insight: **don't build a generic marketing tool**. Build it specifically for developer tool marketing — changelogs as content source, GitHub stars as funnel metric, Discord as community layer, PostHog as analytics backbone. That specificity is what makes it valuable and what no existing tool provides.
