- **severity: MAJOR**  
  **location:** `docs/superpowers/specs/2026-08-04-agent-tui-visual-layout-design.md:96-106, 148-159`  
  **evidence:** The state-contract table specifies required controls only for 60x18 and 60x10, but acceptance criterion 7 requires layout tests for every state at 60x9 and 60x8 as well. The emergency-mode paragraph delegates no-editor row reassignment “according to the state table,” which likewise has no 8–9-row contracts. This leaves palette, pending, recovery, locked, temporary, and unseen rendering at the mandatory 60x8/60x9 test targets underspecified—for example, whether compact palette must retain its filter row, selected item, and help separately, and which normal/recovery controls may be replaced by the single state line.  
  **required change:** Add explicit 60x9 and 60x8 state contracts (or a precisely defined shared 8–9 contract) for every listed state, including reserved rows, required visible controls, allowed omissions/reassignments, and the exact compact palette/pending/recovery/unseen behavior. Update acceptance criterion 7 to map its assertions to those contracts.

VERDICT: NEEDS_CHANGES
