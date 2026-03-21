# Flight Deck — Product Vision

## What is Flight Deck?

Flight Deck is PacketCode's autonomous agentic orchestration layer. It turns PacketCode from a multi-pane AI IDE into a **fully autonomous development pipeline** — you describe what you want, and it plans, builds, and ships the project with minimal human intervention.

A **Flight** is the top-level work unit. It represents an autonomous objective that the system plans, decomposes into issues, executes via AI sessions, and tracks to completion.

## The Agentic Pipeline

```
Ideate → Plan → Build → Ship
```

### 1. Ideate
- User describes a high-level idea or objective
- Ideation engine asks clarifying questions (tech stack, constraints, scope)
- Produces a structured Flight brief with objective, success criteria, and constraints

### 2. Plan
- Flight Deck decomposes the brief into issues with acceptance criteria
- Auto-generates dependencies and work order
- Creates the kanban board automatically
- Estimates complexity and sequences work (what can parallelize, what blocks what)

### 3. Build
- Flight Deck launches Claude/Codex sessions autonomously
- Each session gets a context-rich prompt with the issue details, acceptance criteria, and project context
- Sessions report back on completion — Flight Deck moves issues through the board
- On failure or ambiguity, escalates to "needs human" status
- Next session launches automatically when dependencies are met

### 4. Ship
- Flight Deck validates acceptance criteria are met
- Runs quality checks, tests, linting
- Prepares deploy pipeline
- Flight status moves to "done"

## Key Differentiator

Factory.ai's Droid can run autonomously for 2-3 days in their "Mission Control". PacketCode's Flight Deck aims for the same capability but as a **local-first native desktop IDE** — your code stays on your machine, your agents run locally, and you have full visibility into every session.

## Current State vs. Target

### What exists today (manual, disconnected)
- Ideation scanner generates ideas
- Issue board is manual kanban
- Sessions are manually launched PTY terminals
- Flights (formerly Missions) are a manual orchestration layer
- Status rollup from issues exists but is passive

### What the autonomous flow needs
- Ideation → auto-generates issues with acceptance criteria and dependencies
- Flight auto-plans work order from issue graph
- Sessions self-launch when dependencies clear
- Live status rollup — Flight detects session completion, advances the board, launches next work
- Failure detection — stuck sessions escalate to "needs human" automatically
- Completion validation — acceptance criteria checked before marking done
- The full pipeline from "here's my idea" to "here's your built project" runs hands-off

## Terminology

| Old Term         | New Term        |
|------------------|-----------------|
| Mission          | Flight          |
| Mission Control  | Flight Deck     |
| Missions view    | Flights view    |
