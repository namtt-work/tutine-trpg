# Narrator YAML Output Design

## Goal

Make the narrator's structured LLM response more reliable by requesting and decoding YAML instead of JSON. Retrieval planning and memory extraction remain JSON contracts.

## Scope

`OpenAICompatibleClient.Narrate` changes its output format from JSON to YAML. The narrator request also carries an engine-owned player snapshot and a bounded recent-turn history so the model can continue a scene without re-asking resolved information. Retrieval planning and memory extraction remain JSON contracts.

## Data Flow

`Narrate` sends a prompt requiring only this YAML shape:

```yaml
narration: The scene narration.
npc_dialogue:
  - npc_id: npc_guard
    text: Halt.
proposed_effects: []
suggested_next_options:
  - Present the token.
```

The provider response is trimmed of an optional `yaml`/`yml` fenced block and decoded with `gopkg.in/yaml.v3` into `NarratorResponse`. JSON decoding remains in place for `PlanRetrieval` and `ExtractMemories`.

The narration request contains:

- An authoritative player snapshot: identity, traits, cultivation realm and stage, HP, spiritual energy, combat stats, techniques, artifacts, inventory, relationships, flags, and cooldowns. Internal identifiers are converted to Vietnamese display labels before they reach the narrator.
- The last 12 resolved turns, each with the player action, narration, and engine-accepted state changes.
- Retrieved long-term context and the allowed effect types.

The narrator prompt requires a causal next beat for every turn, bans meta phrasing such as "người chơi", bans repeated questions for information already supplied, and requires Vietnamese xianxia terms. It may not claim an engine-owned scene, quest, reward, or relationship change without authoritative confirmation.

## Errors

An invalid narrator response returns an error prefixed `decode llm yaml`. Provider request, retry, timeout, and HTTP errors retain their existing behavior.

## Tests

- Narrator accepts ordinary YAML.
- Narrator accepts fenced YAML.
- Narrator rejects malformed YAML with the YAML decode error.
- Narrator prompt contains the continuity, Vietnamese-only, and anti-repetition rules.
- The next turn receives the prior resolved narration plus the complete authoritative player snapshot.
- Existing JSON tests preserve planner and extractor behavior.
