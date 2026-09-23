# Review decision and attribution model {#review-decision-and-attribution-model}

## Event history and currency {#event-history-and-currency}

Each review slide receives append-only decisions at a specific source head and slide digest. The current projection keeps the latest decision for each composite reviewer seat while retaining earlier files, then independently marks that decision out of date when the slide or referenced code changes.

## Composite reviewer provenance {#composite-reviewer-provenance}

Reviewer metadata distinguishes a direct human action from a named AI seat with agent and model details. Git attribution supplies the actor who recorded the event: the introducing commit's committer for a committed record, or the local Git user while the record is uncommitted. The review UI renders an AI action as `AI <seat> (<agent>, <model>) for <actor>` and a direct human action as the actor, so the existing pair identifies both the AI and the human principal.

## Attribution degradation {#attribution-degradation}

Attribution is derived rather than copied into the decision event. If Git history was rewritten or cannot be read, the projection exposes `rewritten` or `history_unavailable`; an uncommitted record depends on local Git name and email configuration. These states limit provenance quality but do not block the decision event. This model identifies who recorded the action; it does not authenticate or prove that the human authorized an AI delegation.
