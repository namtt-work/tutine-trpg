# Tutine TRPG Repo Setup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create the initial `tutine-trpg` repository with the approved design spec, README, and git history.

**Architecture:** This setup creates documentation-only project scaffolding. The repo stores the MVP design under `docs/superpowers/specs/` and introduces the product direction in `README.md`; implementation code will be planned separately.

**Tech Stack:** Go runtime planned for future implementation, SQLite FTS planned for memory, OpenAI-compatible APIs planned for LLM calls.

## Global Constraints

- Repository name is `tutine-trpg`.
- CLI is the first adapter, but the game core must remain reusable for web and bot adapters.
- Go is the selected implementation language.
- SQLite FTS is the MVP retrieval layer.
- LLM providers must use an OpenAI-compatible API boundary.
- No game implementation code is added in this setup task.

---

### Task 1: Initial Repository Documentation

**Files:**
- Create: `README.md`
- Move: `docs/superpowers/specs/2026-08-03-tu-tien-llm-rpg-design.md`
- Create: `docs/superpowers/plans/2026-08-03-tutine-trpg-repo-setup.md`

**Interfaces:**
- Consumes: Approved MVP design decisions from the brainstorming session.
- Produces: A git-initialized documentation baseline for later Go implementation planning.

- [x] **Step 1: Create repository folder**

Run: `mkdir -p tutine-trpg/docs/superpowers/specs tutine-trpg/docs/superpowers/plans`
Expected: `tutine-trpg` exists with docs folders.

- [x] **Step 2: Move approved design spec into repository**

Run: `mv docs/superpowers/specs/2026-08-03-tu-tien-llm-rpg-design.md tutine-trpg/docs/superpowers/specs/`
Expected: Spec exists under `tutine-trpg/docs/superpowers/specs/`.

- [x] **Step 3: Write setup plan**

Run: create `docs/superpowers/plans/2026-08-03-tutine-trpg-repo-setup.md` with this checklist.
Expected: Plan documents the repo setup task and its constraints.

- [x] **Step 4: Write README**

Create `README.md` describing `Tutine TRPG`, the architecture direction, MVP scope, planned layout, and development status.
Expected: README gives a clear first view of the project without claiming unfinished code exists.

- [x] **Step 5: Initialize git and commit**

Run:

```bash
git init
git add README.md docs/superpowers/specs/2026-08-03-tu-tien-llm-rpg-design.md docs/superpowers/plans/2026-08-03-tutine-trpg-repo-setup.md
git commit -m "chore: initialize tutine trpg docs"
```

Expected: Initial commit contains README, design spec, and setup plan.
