# Narrator YAML Output Implementation Plan

**Goal:** Decode narrator responses as YAML while preserving JSON contracts for retrieval planning and memory extraction.

## Checklist

- [x] Add focused YAML narrator response tests, including fenced output and malformed output.
- [x] Change `OpenAICompatibleClient.Narrate` to request and decode YAML.
- [x] Preserve existing JSON planner and extractor behavior.
- [x] Run formatting and the full Go test suite.
