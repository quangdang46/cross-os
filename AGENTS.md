<!-- br-agent-instructions-v1 -->

---

## Beads Workflow Integration

This project uses [beads_rust](https://github.com/Dicklesworthstone/beads_rust) (`br`/`bd`) for issue tracking. Issues are stored in `.beads/` and tracked in git.

### Essential Commands

```bash
# View ready issues (open, unblocked, not deferred)
br ready              # or: bd ready

# List and search
br list --status=open # All open issues
br show <id>          # Full issue details with dependencies
br search "keyword"   # Full-text search

# Create and update
br create --title="..." --description="..." --type=task --priority=2
br update <id> --status=in_progress
br close <id> --reason="Completed"
br close <id1> <id2>  # Close multiple issues at once

# Sync with git
br sync --flush-only  # Export DB to JSONL
br sync --status      # Check sync status
```

### Workflow Pattern

1. **Start**: Run `br ready` to find actionable work
2. **Claim**: Use `br update <id> --status=in_progress`
3. **Work**: Implement the task
4. **Complete**: Use `br close <id>`
5. **Sync**: Always run `br sync --flush-only` at session end

### Key Concepts

- **Dependencies**: Issues can block other issues. `br ready` shows only open, unblocked work.
- **Priority**: P0=critical, P1=high, P2=medium, P3=low, P4=backlog (use numbers 0-4, not words)
- **Types**: task, bug, feature, epic, chore, docs, question
- **Blocking**: `br dep add <issue> <depends-on>` to add dependencies

### Session Protocol

**Before ending any session, run this checklist:**

```bash
git status              # Check what changed
git add <files>         # Stage code changes
br sync --flush-only    # Export beads changes to JSONL
git commit -m "..."     # Commit everything
git push                # Push to remote
```

### Best Practices

- Check `br ready` at session start to find available work
- Update status as you work (in_progress → closed)
- Create new issues with `br create` when you discover tasks
- Use descriptive titles and set appropriate priority/type
- Always sync before ending session

<!-- end-br-agent-instructions -->

<!-- bv-agent-instructions-v4 -->

---

## Beads Workflow Integration

This project uses [beads_rust](https://github.com/Dicklesworthstone/beads_rust) (`br`) for issue tracking and [beads_viewer_rust](https://github.com/quangdang46/beads_viewer_rust) (`bvr`) for graph-aware triage. Issues are stored in `.beads/` and tracked in git. Current `br` workspaces normally export `.beads/issues.jsonl`; older `bd`/legacy workspaces may use `.beads/beads.jsonl`. `bvr` auto-discovers the supported JSONL files, so agents should use `br`/`bvr` commands instead of hard-coding a single filename.

### Using bvr as an AI sidecar

bvr is a graph-aware triage engine for Beads projects. Instead of parsing .beads/issues.jsonl / .beads/beads.jsonl directly or hallucinating graph traversal, use robot flags for deterministic, dependency-aware outputs with precomputed metrics (PageRank, betweenness, critical path, cycles, HITS, eigenvector, k-core).

**Scope boundary:** bvr handles *what to work on* (triage, priority, planning). `br` handles creating, modifying, and closing beads.

**CRITICAL: Use ONLY --robot-* flags. Bare bvr launches an interactive TUI that blocks your session.**

#### The Workflow: Start With Triage

**`bvr --robot-triage` is your single entry point.** It returns everything you need in one call:
- `quick_ref`: at-a-glance counts + top 3 picks
- `recommendations`: ranked actionable items with scores, reasons, unblock info
- `quick_wins`: low-effort high-impact items
- `blockers_to_clear`: items that unblock the most downstream work
- `project_health`: status/type/priority distributions, graph metrics
- `commands`: copy-paste shell commands for next steps

```bash
bvr --robot-triage        # THE MEGA-COMMAND: start here
bvr --robot-next          # Minimal: just the single top pick + claim command
```

Before claiming, verify current state with `br show <id> --json` or `br ready --json`. `recommendations` can include graph-important blocked or assigned work; only `quick_ref.top_picks` and non-empty `claim_command` fields represent claimable work.

#### Other bvr Commands

| Command | Purpose |
|---------|---------|
| `bvr --robot-insights` | Deep graph analysis: PageRank, betweenness, HITS, k-core, critical path |
| `bvr --robot-plan` | Dependency-respecting execution plan with parallel tracks |
| `bvr --robot-priority` | Priority misalignment detection |
| `bvr --robot-alerts` | Stale issues, blocking cascades |
| `bvr --robot-suggest` | Smart suggestions: duplicates, missing dependencies, labels |
| `bvr --robot-graph` | Dependency graph export (JSON/DOT/Mermaid) |
| `bvr --robot-search <query>` | Semantic search over issue titles/descriptions |
| `bvr --robot-history` | Bead-to-commit correlation from git history |
| `bvr --robot-label-health` | Per-label health metrics |
| `bvr --robot-schema` | JSON Schema definitions for all robot outputs |

#### br Quick Reference

```bash
br list                          # List all beads
br ready                         # Actionable beads (no open blockers)
br show <id>                     # View bead details
br update <id> --status in_progress  # Claim work
br update <id> --status closed   # Complete work
br dep add <id> <target>         # Add dependency
br sync --flush-only             # Sync changes to issues.jsonl
```
<!-- end-bv-agent-instructions -->

