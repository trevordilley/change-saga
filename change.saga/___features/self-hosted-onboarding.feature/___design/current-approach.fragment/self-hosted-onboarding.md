# Proposed repository setup through onboarding {#proposed-repository-setup}

This design develops the existing proposed onboarding story. It describes an intended UI, not a currently deployed onboarding service or permission to create repositories.

## Make repository roles visible {#repository-roles}

The setup experience should collect and display the target source repository separately from the repository that will hold the Saga. Confirmation must make both roles visible: selecting the documentation repository cannot silently replace the source target. Reopening setup should show the confirmed identities so similarly named repositories remain distinguishable.

A target selection should be checked for accessibility and identity before it is confirmed. Saga-repository setup should likewise report an inaccessible or invalid location without overwriting the existing association. Cancellation should discard the draft choice rather than changing confirmed configuration. These are proposed design safeguards supporting comprehensible setup; they do not establish a credential or access agreement.

## Deliberate publication {#setup-publication}

The intended sequence is select roles, inspect the proposed association, explicitly confirm, and reopen the saved setup. Any partial failure needs an explanation of which configuration, if any, was saved. An implementation must define ownership of repository creation, credentials, cancellation and retries before that flow can be treated as complete. The current manual quality scenarios provide observable checks for successful setup, invalid selections and cancellation while retaining the story's proposed state.
