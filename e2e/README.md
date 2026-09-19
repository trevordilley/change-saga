# Real-browser end-to-end tests

This suite builds and runs the real `change-saga` binary. Every test creates
fresh, separate source and saga Git repositories, commits a deterministic
source comparison, generates exact coverage through the CLI, and starts the
server on an ephemeral loopback port.

## Run locally

Use the Node version in `.node-version`, then run:

```sh
npm ci
npx playwright install chromium
npm test
```

`npm run test:all-browsers` also runs the defined Firefox and WebKit projects.
`npm run test:repeat-critical` repeats the mutation-heavy critical flows three
times to check isolation and timing.

On failure, Playwright retains the trace, screenshot, and video. The fixture
also attaches browser console/network events, server output, and a sanitized
snapshot of both temporary repositories before deleting them.

## Layers

Each check runs at the cheapest layer that can still tell the truth about it.

- **Browser** (`navigation`, `documentation`, `implementation-deck`,
  `accessibility`): a real Chromium page against the real server process.
  Reviewer behavior, rendering, focus, and axe scans live here, including the
  guarantee that the Saga is documentation with no approval, comment, or
  annotation control in observe or compare mode.
- **HTTP against the running process** (`security`): raw Node requests to the
  spawned server, which is the only way to forge the `Host` and `Origin`
  headers a browser refuses to send. Used for the removed write endpoints,
  cross-origin, code-location, and path-leak checks.
- **Subprocess** (`cli`, plus the non-loopback check in `security`): the real
  binary invoked with real arguments, asserting exit status, message, and that
  a refusal wrote nothing.

## Fixtures

- `saga`: both repositories, a running server, and a page already loaded.
- `sagaRepositories`: both repositories only, with no server and no browser, for
  subprocess tests.

Both hand every subprocess a private `TMPDIR` inside the fixture, so nothing
the CLI or server writes to temporary storage escapes it. `startSagaServer`
compares against `main` by default; pass `null` to observe the head instead.

The fixture source repository has an `origin` matching the declared repository
URI, because the CLI verifies declared identity against the checkout's origin on
every read. `identity` on the fixture carries the repository URI and the merge-base and head
commits. Evidence is authored through the real `cover` CLI, which pins each code
reference to a commit and digests its content. `codeLocation` builds the one
canonical `<commit>:<path>[#L<start>[-L<end>]]` spelling the product accepts, and
`codeDigest` recomputes a reference's `sha256:` digest from `git show`, so a test
can assert stored references byte for byte. A positive control in
`security.spec.ts` asserts the location spelling is exactly what the server
itself renders, so the malformed and non-canonical cases cannot pass vacuously.

## Zero-side-effect assertions

`treeSnapshot()` records every path and size under a directory. A rejection test
snapshots the saga before the rejected requests and compares afterwards, so a
partial write, a stray lock file, or a rewritten record all fail the test.

## Accessibility

`expectNoSeriousAccessibilityViolations(page)` scans the whole page — chrome
included — for serious and critical axe violations.

Every workspace view is scanned without exclusions or disabled rules and must
have no serious or critical axe violations.
