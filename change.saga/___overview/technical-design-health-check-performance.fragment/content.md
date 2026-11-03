`check --covers health` now skips unrelated review reports and next-action
suggestions. It still loads and validates the Saga, resolves the current source
commit, and reassesses every health finding on every invocation. The check JSON
contract and exit codes are unchanged.

## Health-only evaluation {#health-only-evaluation}

A check passes its requested areas into status assembly, so a health-only invocation
skips unrelated review reports and next-action suggestions while retaining the
same health-area accounting and report contract.

The expensive reusable part is the source comparison behind code references.
`gitdiff.TreeChanges` now persists successful file-change and hunk geometry
through `internal/snapshotcache`, shared by separate CLI processes. It does not
persist a healthy/unhealthy flag in the Saga.

## Reference-scoped cold comparisons {#reference-scoped-cold-comparisons}

Coverage also declares its referenced paths before resolving them. Git still
detects renames across the whole comparison, but constructs patches only for
the files those references need. Independent comparisons run with at most four
workers per resolver; separate agent processes each have their own bound. Selection-specific patch commands exit after each bounded argument
chunk, so many historical pins do not accumulate idle Git processes. Type
changes retain the complete-comparison fallback.

## Measurements {#measurements}

Measured on 2026-11-02 with Go 1.26.1, darwin/arm64, against the supplied
`spike-change-saga-technical-design/change.saga`, observing source commit
`4dac8eae6cb7a24d34ae31f38024dd64d1f3f283`:

| Invocation | Wall time |
| --- | ---: |
| Original implementation | 17.56 s |
| Updated implementation, empty application cache | 17.49 s |
| Updated implementation, repeated invocation | 5.68 s |
| Updated implementation, next repeated invocation | 5.09 s |

Those are the initial comparison-cache measurements. A follow-up on 2026-11-03
addressed the cold path as well, using fresh cache roots on the same Saga:

| Implementation | Empty application cache | Repeated invocation |
| --- | ---: | ---: |
| Initial comparison cache | 17.58 s | 7.45 s |
| Selected-file patches, serial comparisons | 12.60 s | 6.24 s |
| Selected-file patches, bounded concurrency | 8.85 s | 6.48 s, then 5.26 s |

The last group overlapped focused CLI tests, so this is evidence of the cold
path improvement, not a controlled claim about worst-case latency. Every result
again matched the original JSON byte for byte. These measurements do not
establish a latency bound for a different Saga that currently takes 120 seconds.

After the full suite finished, a fresh sequential control run of the original
binary and final `bin/change-saga` measured:

| Implementation | Empty application cache | Repeated invocation |
| --- | ---: | ---: |
| Original implementation | 19.90 s | 15.57 s |
| Final selected-patch implementation | 6.54 s | 4.21 s |

All four outputs again matched the original JSON byte for byte. This directly
tests the cold algorithm as well as cache reuse; host load can still change
absolute timings.

The initial four runs used prebuilt binaries sequentially with a warm filesystem; the
updated runs shared a fresh `CHANGE_SAGA_CACHE_DIR`. The original implementation
also exceeded a 50-second deadline earlier in the session. Filesystem warmth
and concurrent workload materially affect these times, so they are measurements
of this fixture and environment, not latency guarantees.
A later check through the initial comparison-cache implementation, while the full test suite
was running, took 46.74 seconds with the default cache initially empty and
11.59 seconds on its next invocation. Both also returned the same JSON. Cold
checks and checks under concurrent load can therefore still exceed an agent's
short timeout.

The four initial runs returned byte-identical JSON and exit code 3:
**5,974 examined records, 5,668 healthy, 306 unhealthy**. The earlier recorded
“zero stale” result is not the current health result. Unhealthy records are
findings to reassess; speeding up the check does not make them healthy.
All 306 flagged records in this run are prototype annotations, with reasons
including changed prototype and story revisions.

Profiling found two independent costs: repeatedly generating repository-wide
patches for historical code pins, and evaluating pull-request review ranges
even when the command asked only about health. A trial that compared unchanged
Git tree entries before diffing did not improve end-to-end time on this fixture
and was removed.

## Uncached scaling experiment {#uncached-scaling-experiment}

`BenchmarkSelectedChanges` holds one small referenced file fixed while growing
an unrelated changed file. It reads Git afresh on every iteration, bypassing
the application cache. A three-iteration diagnostic run measured:

| Unrelated changed lines | Full patches | Selected patches | Full Go allocation/op | Selected Go allocation/op |
| --- | ---: | ---: | ---: | ---: |
| 10,000 | 43.3 ms | 54.8 ms | 3.30 MB | 0.34 MB |
| 100,000 | 269.1 ms | 41.3 ms | 31.60 MB | 0.35 MB |
| 1,000,000 | 1,771.4 ms | 32.4 ms | 299.84 MB | 0.35 MB |

These allocations cover comparison processing, not Saga loading, cache-key
validation, Git child-process memory, or whole-command peak RSS. Timings vary
with load and garbage collection; the durable result is that unrelated patch
text no longer causes proportional Go allocation. Small comparisons can pay
extra fixed overhead for the metadata pass. Global rename detection still
depends on repository metadata, and large *referenced* files still need their
patches read.

Reproduce with:

```sh
go test ./internal/gitdiff -run '^$' -bench '^BenchmarkSelectedChanges$' -benchtime=3x -count=1
```

## What makes a cached comparison reusable {#what-makes-a-cached-comparison-reusable}

Each key covers the repository path, both full commit IDs, the selected path
set when applicable, the derived-format
version, Git version and environment, Git configuration including included
files, history overrides (replace refs, grafts, shallow boundaries), and the
effective Git attributes for paths in both commit trees. Git itself evaluates
attributes, covering nested and untracked `.gitattributes`, index fallback,
global attributes, and `info/attributes`. Timestamp preservation cannot hide an
attribute edit.

Ordinary loose branch refs are not scanned for these keys: the comparison
already names full commit IDs. This matters in repositories with many agent
branches. Packed refs are still covered because they may contain replacements.

Commit availability is checked even on hits. Missing pins and failed reads are
not saved as settled comparisons. Inputs are checked again before publishing a
new generation, and publication uses the existing atomic staging/rename
mechanism. A checksum detects damaged saved geometry; missing, corrupt, or
unwritable cache data falls back to a fresh comparison. Cache maintenance keeps
the newest 128 generations per repository and preserves recent staging
directories belonging to concurrent builders; staging debris is retired after
24 hours.

Source or Git-input changes select a different comparison key. Saga edits are
loaded and reassessed independently, so even a cache hit can produce a different
health result after a reference or relation is edited. Deleting the cache
affects performance only. `CHANGE_SAGA_CACHE_DIR` redirects its location; the
default remains the existing user-cache snapshot index, outside the Saga.

This is comparison caching, not incremental loading of all Saga JSON. Repeated
checks still pay for Saga loading, validation, attribute checks, and health
assembly. Further reductions should be profiled against those costs.

## Regression verification {#regression-verification}

Regression coverage checks reuse across sessions, source and Saga edits,
attribute and driver configuration changes, history replacement, pruned pins,
corrupt or unavailable caches, changes during a build, and equivalence of
focused health with full status in observing and comparison modes.
Selected-patch regressions additionally compare geometry with complete Git
patches for renames with reoccupied old paths, modes, binary attributes, quoted
paths, and type changes. They check argument chunking, expanded and unplanned
reference paths, streaming walks, inherited references, parallel resolution,
missing pins, tampered digests, and cancellation.

## Source and verification provenance {#source-and-verification-provenance}

The implementation and its regression tests are committed at
`5799fdee43a2184202891f68568ffe51886c1650`. The page's landmarks pin the relevant
implementation and regression code at that commit. The full Go suite, focused
race checks and `go vet ./...` passed on those source changes before the source
commit; the commit introduced no further implementation changes. Measurements
above concern the built binaries described with each experiment. A passing
Saga health check verifies reference currency, not semantic correctness or a
latency guarantee for every Saga.
