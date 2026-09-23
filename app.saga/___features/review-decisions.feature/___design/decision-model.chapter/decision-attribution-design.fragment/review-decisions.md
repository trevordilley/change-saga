# Review decision and attribution model {#review-decision-and-attribution-model}

Each review slide receives append-only decisions at a specific source head and slide digest. The current projection keeps the latest decision per reviewer seat while retaining earlier files, and marks it out of date when the slide or referenced code changes.

Git identifies who committed an event. Reviewer metadata distinguishes direct human actions from named AI seats with agent and model details. The current schema does not explicitly name a human principal for an AI acting on that person's behalf, so delegated authority remains only partially represented.
