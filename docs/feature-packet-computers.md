# Packet Computers — Registry (Milestone A)

A **Packet Computer** is a durable machine packetcode can eventually delegate
work to. This document describes what ships today.

## What works today

The registry only. packetcode stores machine records, lists them, and shows
their detail. That is the whole feature.

```text
/computers                  list registered computers
/computers status <name>    show one computer's stored record
/computers <name>           same as `status <name>`
```

## What does not work yet

Stated plainly, because a registry that looked like a control surface would be
misleading:

- packetcode **cannot run any work** on a registered computer. There is no
  `/spawn --computer`, and jobs carry no computer identity.
- There is **no daemon** and no transport. Nothing connects to anything.
- Status is a **stored value, not a probe**. With no heartbeat, a record that
  has never been contacted reports `unknown` — which means "never contacted",
  not "offline".
- There are **no write commands yet**. Registration is done by editing
  `registry.json` directly until PCMP3 lands.

Roadmap: [`packet-computers-loop.md`](packet-computers-loop.md) (PCMP1–PCMP9).
Product definition and the full six-phase arc:
[`../PACKETCOMPUTERS.md`](../PACKETCOMPUTERS.md).

## Storage

`~/.packetcode/computers/registry.json`, honouring `PACKETCODE_HOME` like every
other state directory. Writes are atomic (temp file then rename).

A malformed file, or one written by a newer packetcode, is a **loud error**
rather than a silent reset — quietly losing your machine list would be worse
than refusing to start the feature.

## Record shape

```json
{
  "version": 1,
  "computers": [
    {
      "id": "pc_workstation",
      "name": "workstation",
      "kind": "ssh",
      "status": "unknown",
      "os": "linux",
      "arch": "amd64",
      "project_roots": ["/srv/projects"],
      "ssh_host": "build.example.internal",
      "ssh_user": "ian",
      "ssh_port": 22,
      "capabilities": {
        "shell": true,
        "filesystem": true,
        "jobs": true,
        "terminals": false,
        "browser": false
      },
      "policy": {
        "network": "ask",
        "write": "ask",
        "shell": "ask",
        "secrets": "deny",
        "approval": "explicit"
      },
      "created_at": "2026-07-31T00:00:00Z"
    }
  ]
}
```

`kind` is `local`, `ssh`, or `managed`. `managed` records are accepted but
nothing provisions them.

Names are unique case-insensitively so `/computers <name>` is unambiguous, and
are restricted to letters, digits, `_`, and `-`.

**No credentials live in this file.** SSH host, user, and port describe how a
machine would be reached; keys and passwords are deliberately absent, and
Milestone A never connects anyway.

## Policy defaults

Any record that does not specify a policy — or specifies an unrecognised value
— gets the conservative default:

| Axis | Default | Meaning |
|---|---|---|
| `network` | `ask` | Network access is approved per use. |
| `write` | `ask` | Filesystem writes are approved per use. |
| `shell` | `ask` | Shell execution is approved per use. |
| `secrets` | `deny` | A remote computer does not inherit local credentials. |
| `approval` | `explicit` | Every action is approved explicitly. |

`approval` accepts `explicit`, `trust-workspace`, or `trust-computer`. It is a
string enum rather than a boolean on purpose: the safe value is "explicit", and
a boolean whose safe value is `true` would decode an absent JSON field as
`false` — silently widening trust on exactly the records nobody configured. An
unrecognised value falls back to `explicit` for the same reason.
