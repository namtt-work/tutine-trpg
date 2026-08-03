# Task 1 Report

Status: DONE

## Files Changed

- `go.mod`
- `internal/game/state.go`
- `internal/game/effects.go`
- `internal/game/effects_test.go`

## Tests Run

- `go test ./internal/game`: PASS
- `go test ./...`: PASS
- `git diff --check`: PASS

## Commits Created

- `a451a26` `feat: add core game state and effects`
- `docs: add task 1 report` (final commit on this branch)

## Self-Review Notes

- Starter saves initialize independent maps and slices and use UTC timestamps.
- Unknown items and invalid item amounts are rejected before item mutation.
- Relationship deltas are clamped to the required range and returned in the state-change view.

## Concerns

- No concerns for the scope of Task 1. The starter allowlist is intentionally local and should move to campaign configuration in a later task.
