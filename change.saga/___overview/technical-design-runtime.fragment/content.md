# Runtime and trust {#runtime-and-trust}

The [Infrastructure board](/technical/whiteboards/infrastructure) documents the local application on `main`. One Go binary provides CLI commands and the optional browser server. Node and Playwright are development and browser-test tools, not required runtime services for the installed CLI.

## Workstation boundary {#workstation-boundary}

The terminal runs the binary with access to the user's source checkout and Saga. Git supplies repository identity, commits, diffs and historical file content. The Saga may live in the source repository or in a companion repository; commands must select the intended source checkout in either case. The local browser connects over loopback HTTP to server-rendered pages and bounded detail endpoints.

The HTTP listener refuses non-loopback addresses. Host validation and mutation-token checks protect the local request boundary; loopback access is not a multi-user account system. File and asset paths remain confined to their allowed roots. Markdown and visuals are treated as authored content with rendering restrictions, including sandboxed HTML where supported.

## Durable and disposable resources {#durable-and-disposable-resources}

Git commits and Saga records are durable. Read-session caches and generated comparison partitions live under the user's cache directory outside the Saga. They are keyed by explicit inputs and may be deleted and rebuilt. A worktree view that cannot provide a stable source identity does not gain a persistent cache entry that pretends to be committed truth.

## Build and distribution {#build-and-distribution}

Go builds the command-line application. Repository CI runs Go checks and browser tests, and release workflows package binaries and their checksums. Users download a release or build from source. GitHub is a source and distribution service in this design, not a required application server for reading an already available checkout. The deferred hosting/onboarding branch is outside this description. [Return to the approach](/technical).
