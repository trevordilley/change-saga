# Safe local rendering contract {#safe-local-rendering}

Reviewing a branch treats its authored content as untrusted input. The reviewer
application may explain local source and render rich Saga artifacts, but those
artifacts never inherit the authority of the host process, the parent page, or
the reviewer's network identity.

## Entry boundary {#entry-boundary}

The server binds only to loopback and rejects requests whose Host or browser
site context does not match the local origin. It does not infer trust merely
because a request reached a local port. Startup fails closed when the requested
address would expose the reviewer beyond the machine.

URLs and errors presented to the browser use Saga-relative identities. Local
repository roots, home directories, and temporary paths remain server-side
diagnostics.

## Authored-content boundary {#authored-content-boundary}

Markdown and text become inert application-owned markup. Raster images are
decoded as data. SVG and interactive HTML render in a separate sandboxed
document with no same-origin privilege, parent access, top navigation,
downloads, popups, forms, or network capability. A restrictive content policy
blocks remote scripts, styles, fonts, media, frames, and connections.

The parent application communicates through a narrow message contract that
accepts only expected origin, source window, message kind, target identity, and
bounded payload. Authored content cannot synthesize review mutations or read
application state.

## Resource and failure boundary {#resource-boundary}

Requests, decoded bodies, rendered responses, archive entries, diff expansion,
and concurrent work have explicit limits. Limit failures produce a stable,
actionable placeholder and never fall back to an unsafe renderer. A failed or
partial artifact is not marked viewed.

Unknown media, malformed markup, stale evidence, and unavailable source are
distinct states. Each identifies the affected artifact, preserves surrounding
navigation, and routes diagnostics without exposing local paths.

## Offline guarantee {#offline-guarantee}

All runtime code, fonts, syntax assets, and rendering dependencies ship locally.
Normal review performs no DNS lookup or outbound request. External references
are displayed as inert destinations and open only after an explicit reviewer
action outside the authored-content sandbox.
