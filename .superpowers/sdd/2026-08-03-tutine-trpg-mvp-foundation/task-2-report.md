# Task 2 Report

Status: DONE

## Files changed

- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/campaign/campaign.go`
- `internal/campaign/campaign_test.go`
- `configs/example.yaml`
- `campaigns/thanh-van-sect/campaign.yaml`
- `campaigns/thanh-van-sect/tags.yaml`
- `go.mod`
- `go.sum`

## Tests

- `go test ./internal/config ./internal/campaign` - PASS
- `go test ./...` - PASS
- `git diff --check` - PASS

## Commits

- `45cbff3 feat: add config and campaign loading`

## Self-review

- Loaders use YAML structs and return file or parse errors without mutating game state.
- Campaign tags are retained by category and indexed for exact vocabulary checks.

## Concerns

None.
