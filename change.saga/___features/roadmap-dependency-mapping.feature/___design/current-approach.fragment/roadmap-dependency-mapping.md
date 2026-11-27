# Proposed directed Roadmap prerequisites {#roadmap-prerequisites}

This is a design proposal for communicating dependencies between planned capabilities. It does not claim that a public feature-dependency command, scheduler or hosted deployment exists.

The concrete example is self-hosted onboarding depending on self-hostable server mode. The representation should retain two stable feature identities with an explicit direction: onboarding is the dependent and server mode is the prerequisite. A reader should be able to inspect that direction from the explanation and follow each canonical feature page without confusing display order with dependency order.

A feature prerequisite is different from a Task dependency inside one Plan, from a shared milestone, and from a Roadmap feature augmenting another feature's product intent. Those existing concepts must not be repurposed to make a prerequisite appear implemented. The durable relation representation, authoring operation and presentation still require implementation decisions. Handling cycles, removal and unresolved feature identities should be specified before a general dependency editor is claimed.

The verification target is deliberately narrow: record the example relation through the eventual supported authoring path and confirm a reader can identify which capability must precede the other. Definition coverage records that planned check; it does not supply scheduling or delivery proof.
