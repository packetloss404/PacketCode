# Flight Deck — Implementation Plan

## Phase 1: Foundation (Current — manual orchestration)
**Status: Done**

- [x] Flight (Mission) CRUD — create, edit, delete, status, priority
- [x] Issue linking — assign issues to flights, bidirectional sync
- [x] Session launch from flight — context-rich prompts with objective + linked issues
- [x] Session auto-linking — sessions launched from a flight are tracked
- [x] Flight Deck dashboard — status strip, attention queue, active flights grid
- [x] Board integration — flight badges on issue cards, flight filter on kanban
- [x] Status rollup — flight status computed from linked issue states
- [x] Inline editing — edit title, objective, status, priority in detail panel

## Phase 2: Semi-Autonomous (Next)

### Auto-Planning
- [ ] Ideation → Flight: "Create Flight from idea" button that generates a Flight brief
- [ ] Flight brief → Issues: auto-decompose objective into issues with acceptance criteria
- [ ] Auto-dependency detection: analyze issues and set blockedBy/blocks relationships
- [ ] Work order generation: topological sort of issue graph, identify parallelizable work

### Auto-Session Management
- [ ] "Auto-pilot" toggle on a Flight — when enabled, Flight Deck launches sessions without user intervention
- [ ] Session completion detection: watch PTY output for session exit, parse exit code
- [ ] On session complete: mark issue done, check what's unblocked, launch next session
- [ ] On session failure: mark issue blocked, escalate flight to "needs_human"
- [ ] Parallel sessions: launch multiple sessions for independent issues simultaneously

### Progress Tracking
- [ ] Live progress bar on Flight cards (issues done / total)
- [ ] Time tracking per session and per flight
- [ ] Session output summaries (what changed, what was built)

## Phase 3: Fully Autonomous

### Intelligent Planning
- [ ] Clarifying questions: ideation engine interviews user before generating plan
- [ ] Scope estimation: estimate complexity per issue, flag overly ambitious flights
- [ ] Re-planning: if a session fails, analyze why and adjust the remaining plan
- [ ] Context accumulation: each session gets context from all prior sessions in the flight

### Quality Gates
- [ ] Acceptance criteria validation: run checks after each session
- [ ] Automated testing: trigger test suite after code changes
- [ ] Lint/type-check gates: block issue completion if quality checks fail
- [ ] Code review step: AI reviews its own changes before marking done

### Completion & Deploy
- [ ] Flight completion checklist: all issues done, all criteria met, all checks pass
- [ ] Auto-trigger deploy pipeline on flight completion
- [ ] Flight summary report: what was built, time taken, sessions used, issues resolved

## Phase 4: Fleet Operations

### Multi-Flight Management
- [ ] Priority queue: flights compete for session slots based on priority
- [ ] Resource limits: max concurrent sessions across all flights
- [ ] Flight templates: reusable flight patterns (e.g., "add feature", "fix bug", "refactor module")
- [ ] Flight cloning: duplicate a flight structure for similar work

### Observability
- [ ] Flight timeline: visual history of session launches, completions, escalations
- [ ] Cost tracking: token usage per session, per flight, per time period
- [ ] Performance metrics: average time-to-completion by flight type
- [ ] Alert system: notifications for escalations, completions, failures
