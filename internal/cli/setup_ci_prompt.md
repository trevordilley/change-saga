# Set up CI and review ownership for a repository's Saga

Use this guide with a coding agent or repository maintainer. This command only
prints guidance: it does not edit files, contact the hosting provider, change
branch protection, or grant approvals. GitHub Actions and GitHub review rules
are the worked example; adapt the same policy to other providers.

Default: changes that only touch the root change.saga invoke no CI and have no
CODEOWNERS. That includes product, design, quality, and shared Saga records.
Do not add a Saga validation job, classifier job, required gate, or review owner
unless the user opts in. Mixed changes still follow the normal CI and ownership
rules for files outside change.saga. CI checks and Saga ownership are independent
options: choosing one does not imply choosing the other.

This is the recommended setup policy, not something the CLI can enforce merely
by printing it. Existing repository automation and host rules need configuring.

## Inspect existing configuration

Locate the canonical Saga. The recommended layout is one change.saga at the
repository root, including in a monorepo. Preserve an existing different location
and substitute its exact repository-relative path in the examples. Read existing
workflows, affected-project configuration, CODEOWNERS, effective branch rules,
organization rules, merge queues and deployment triggers before preparing changes.

Start with the exclusions below. Add checks or owners only when requested. If a
build or publishing task consumes the Saga, explain that dependency: automatically
running it for Saga-only changes is an opt-in exception too. Preserve normal checks
and owners for application code and mixed changes. Identify host settings separately;
printing configuration does not apply it.

## Default: no CI for Saga-only changes

For existing GitHub Actions workflows triggered by PRs and pushes, incorporate
this trigger fragment, preserving the repository's existing branch filters:

```yaml
on:
  pull_request:
    paths-ignore:
      - 'change.saga/**'
  push:
    paths-ignore:
      - 'change.saga/**'
```

Apply the exclusion to every relevant workflow, including separate documentation,
post-merge and deployment workflows. Do not create a Saga-only workflow or a
successful no-op job in its place. If a workflow already has paths filters,
combine the exclusion with those filters using the provider's supported syntax;
GitHub does not allow paths and paths-ignore on the same event. A change outside
change.saga must still trigger the normal applicable checks. Review renames
across the directory boundary as mixed changes, including the old path.

Provider limits matter: GitHub's path filters do not apply to tag pushes, have
diff-size limits, and can run workflows when a diff cannot be generated. A
merge_group trigger is not excluded by the PR/push filters above. Check merge
queue requirements and other event types separately; do not claim this fragment
guarantees zero runs for them. Keep release/tag behavior as a separate policy.

Required status checks may remain Pending when their workflow is skipped.
Branch-wide rules cannot be made path-conditional just by adding paths-ignore.
Resolve conflicting requirements with the repository maintainer; do not remove
code protections or silently install an always-running gate as the default.
If the host cannot retain the desired code protections while allowing zero CI
on Saga-only PRs, explain the limitation. The optional gate below is an explicit
tradeoff that runs CI, so it does not satisfy the zero-CI default.

## Default: no CODEOWNERS for the Saga

If there is no CODEOWNERS file, do not create one just for Change Saga. If broad
patterns already assign owners, add an ownerless exclusion after every rule
that could match the Saga. For example, preserve the existing application owner
and end with:

```text
* @example/engineering
# No owners for any file under the root Saga.
/change.saga/
```

The final line intentionally has no owner. In GitHub CODEOWNERS, the last
matching rule wins; simply omitting Saga-specific entries would still inherit
a wildcard owner. Do not use a leading ! as an exclusion: GitHub CODEOWNERS does
not support that syntax. Review/remove any later Saga-matching owner assignments.
The example team is a placeholder for the existing application owner, not an
instruction to add one. Mixed PRs retain ownership of their non-Saga files.

An ownerless entry does not bypass branch-wide review counts, organization
rules, or separately configured required teams. Audit those policies too and
report any remaining restrictions; no CODEOWNERS is not a promise that the host
allows merging without any review under its other rules.

## Opt-in: checks and a required gate

Use this section only when the user chooses CI checks. This example runs CI on
Saga-only changes, including validation and a gate; it is not the default setup.

On GitHub, a required workflow skipped by paths-ignore can leave its check
Pending and block merging. Keep a small workflow running for every relevant PR,
classify all changed paths, and conditionally run expensive application jobs.
Require a stable final gate that verifies its dependencies. A job-level skip
alone does not prove that classification or another dependency succeeded.

Integrate this pattern into the repository's existing jobs. The application
step deliberately fails until replaced with the repository's actual setup and
test/build commands; do not replace it with a successful no-op. Pin the Saga
release to one that reads the repository's format. The installer below is pinned
to the source commit of v0.5.0, rather than downloaded from the PR branch.

```yaml
name: Repository checks
on:
  pull_request:
  merge_group:
permissions:
  contents: read
jobs:
  changes:
    runs-on: ubuntu-latest
    outputs:
      application: ${{ steps.classify.outputs.application }}
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
          ref: ${{ github.event.pull_request.head.sha || github.event.merge_group.head_sha }}
      - id: classify
        env:
          BASE_SHA: ${{ github.event.pull_request.base.sha || github.event.merge_group.base_sha }}
          HEAD_SHA: ${{ github.event.pull_request.head.sha || github.event.merge_group.head_sha }}
          EVENT_NAME: ${{ github.event_name }}
        shell: bash
        run: |
          python3 - <<'PY'
          import os, subprocess
          base, head = os.environ['BASE_SHA'], os.environ['HEAD_SHA']
          if not base or not head:
              raise SystemExit('Missing comparison commits; refuse to skip application CI')
          # PR changes since the common ancestor; merge queue changes in its group.
          separator = '...' if os.environ['EVENT_NAME'] == 'pull_request' else '..'
          paths = subprocess.check_output([
              'git', 'diff', '--no-renames', '--name-only', '-z',
              base + separator + head, '--'
          ]).split(b'\0')
          paths = [path for path in paths if path]
          # No rename folding: moving code into the Saga still includes its old path.
          application = not paths or any(not p.startswith(b'change.saga/') for p in paths)
          with open(os.environ['GITHUB_OUTPUT'], 'a') as output:
              output.write('application=' + str(application).lower() + '\n')
          PY
  application:
    needs: changes
    if: needs.changes.outputs.application == 'true'
    runs-on: ubuntu-latest
    steps:
      - run: |
          echo 'Replace this step with the existing checkout, toolchain and application checks.'
          exit 1
  saga:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
          ref: ${{ github.event.pull_request.head.sha || github.event.merge_group.head_sha }}
      - name: Install the pinned Saga reader
        shell: bash
        run: |
          curl -fsSL https://raw.githubusercontent.com/trevordilley/change-saga/2a7f0e71d0dc8eb7b32f9ad5fb87e7de9f796f70/scripts/install.sh -o "$RUNNER_TEMP/install-saga.sh"
          sh "$RUNNER_TEMP/install-saga.sh" --repo trevordilley/change-saga --version v0.5.0 --dir "$RUNNER_TEMP/saga-bin"
          echo "$RUNNER_TEMP/saga-bin" >> "$GITHUB_PATH"
      - run: change-saga validate change.saga
      - run: change-saga check --covers health change.saga
  gate:
    name: Repository gate
    if: always()
    needs: [changes, application, saga]
    runs-on: ubuntu-latest
    env:
      CHANGES: ${{ needs.changes.result }}
      RUN_APPLICATION: ${{ needs.changes.outputs.application }}
      APPLICATION: ${{ needs.application.result }}
      SAGA: ${{ needs.saga.result }}
    steps:
      - shell: bash
        run: |
          test "$CHANGES" = success && test "$SAGA" = success || exit 1
          if [ "$RUN_APPLICATION" = true ]; then
            test "$APPLICATION" = success
          elif [ "$RUN_APPLICATION" = false ]; then
            test "$APPLICATION" = skipped
          else
            exit 1
          fi
```

Make Repository gate the required status check after verifying the adapted
workflow. Remove or replace obsolete requirements for application jobs that no
longer run on Saga-only PRs; leaving them required defeats the separation. If
there are multiple application jobs, the gate must check every required job's
result. Do not use continue-on-error for required work. Never run untrusted PR
code using pull_request_target or a privileged token to implement this pattern.

The classifier uses the full Git diff, NUL-separated paths, and both sides of
renames rather than a truncated changed-files API response. A Git failure fails
classification and therefore the gate. Empty diffs conservatively run the app.
Run Saga health on code-only PRs too: code edits can stale existing evidence.
Health validates documentation currency, not test execution or human approval.
Require review/implementation coverage only when the team explicitly wants it.

When adopting this optional gate, remove the default path exclusions from this
workflow so its required check can report on every relevant PR. Monorepo
affected-project tools may still exclude the Saga from irrelevant global inputs.

## Opt-in: route reviews using existing paths

Use this section only when the user chooses Saga owners. Replace or override the
ownerless Saga exclusion intentionally; do not apply these examples by default.

The current format already separates many concerns. No directory migration is
needed for these GitHub CODEOWNERS examples. Put them after broad repository
defaults: the last matching CODEOWNERS pattern wins, not all matching lines.
The handles below are placeholders for teams in the repository's organization
with the required access.

```text
# Example opted-in policy: application owners, then product owners for the Saga.
* @example/engineering
/change.saga/ @example/product-owners

# Technical and quality overrides across all feature packages.
/change.saga/___features/*.feature/___design/ @example/architecture
/change.saga/___features/*.feature/___slides/ @example/engineering
/change.saga/___features/*.feature/___workplan/ @example/engineering
/change.saga/___features/*.feature/___quality/ @example/quality
/change.saga/___features/*.feature/___requirements/relations/ @example/architecture
/change.saga/___features/*.feature/___requirements/coverage-exceptions/ @example/quality
/change.saga/___whiteboards/ @example/architecture
/change.saga/___inventory/ @example/architecture
/change.saga/___overview/technical-design*.fragment/ @example/architecture
/change.saga/___overview/data-model.fragment/ @example/architecture

# Ownership policy and automation remain maintainer-reviewed.
/.github/CODEOWNERS @example/repository-maintainers
/.github/workflows/ @example/repository-maintainers
```

Enable required code-owner review on the protected target branch. CODEOWNERS
alone requests review; it does not block merging. Listing two teams on the same
line allows an approval from either owner; it does not require both teams.
Rules elsewhere that require architecture or lead approval on every PR still
apply. Adjust those rules to their intended paths; a product-owned directory
does not exempt a PR from branch-wide approval counts or organization policies.

## Opt-in: require distinct domains or roles

GitHub rulesets also offer Required reviewers: a team, its file patterns, and
its minimum approval count. On organization-owned repositories where available,
configure separate entries for each team that must approve matching changes.
For example, requirements for checkout can require its product team, checkout
design can require its architecture team, and quality can require its QA team.
Overlapping entries can require both product and security for the same paths;
this is different from putting both owners on one CODEOWNERS line. The team
must have write access or higher. This option is not available for user-owned
repositories; the REST API currently labels required_reviewers beta. Verify
the host's actual availability and pattern behavior before relying on it.

Example domain paths (substitute an existing feature ID):

- Product: /change.saga/___features/checkout.feature/___requirements/stories/**
- Design: /change.saga/___features/checkout.feature/___design/**
- Quality: /change.saga/___features/checkout.feature/___quality/**
- Whole domain: /change.saga/___features/checkout.feature/**

Specify every feature path belonging to a domain. Features live flat under
___features; themes, labels, milestones, whiteboard roles and relations are
metadata, not directory-based permission boundaries. CODEOWNERS cannot inspect
JSON or require review based on a label. A feature move or rename needs a policy
review too, including both old and new paths. A whole-domain override placed
after role overrides in CODEOWNERS replaces them; use explicit intersections
or separate Required reviewers entries when both roles must approve.

Some files mix concerns: a feature's identity/plan, theme membership, shared
technical inventory, overview prose, cross-domain relations and review records.
Assign a deliberate owner for each shared location. Do not classify all of
___requirements as exclusively product: relations and coverage exceptions can
alter technical evidence or policy. Keep a default owner for new/unmapped files.
Observe all files a representative CLI edit changes before setting fine-grained
rules; do not claim that a logical edit touches only one domain.

Directory changes alone cannot create all-team approval semantics on a host
that lacks them. GitLab has separately enforced Code Owner sections (subject
to its tier/settings); other hosts may need their own approval rules or a
trusted required review-policy check. Such a check must evaluate the base
branch's policy and current approvals, not policy supplied by the PR. Moving
canonical Saga records into arbitrary domain folders would be a format change,
with reader/writer/schema and identity implications; do not move them as CI setup.

## Verify before declaring setup complete

First verify the default policy against the configured target branch:

| Change | Default expectation |
| --- | --- |
| Only Saga product, design, quality or shared records | No CI runs; no CODEOWNERS |
| Saga plus code or workflow files | Normal applicable CI and owners for non-Saga files |
| Move code into/out of the Saga; delete code | Normal application checks and code ownership |
| Existing wildcard CODEOWNERS rules | The final ownerless Saga rule overrides them |
| Existing required checks or reviews; merge queue | Identify host constraints before claiming the default works |

Only if the user opted into checks and ownership, verify the selected policy
using the cases below. Do not use these expectations for the default setup:

| Change | Expected work and review |
| --- | --- |
| Only a product story in change.saga | Saga checks; product owner; application jobs skipped |
| Only technical design | Saga checks; configured technical owner |
| Only quality definitions | Saga checks; configured quality owner |
| Application code only | Existing application checks and Saga health; code owners |
| Saga plus code or workflow files | Both checks; all applicable review rules |
| Move code into/out of the Saga; delete code | Application checks still run |
| Invalid or deleted Saga | Saga check and final gate fail |
| Classifier failure; application failure/cancellation | Final gate fails |
| New feature, shared record, cross-domain edit | Default/shared owners or all configured domain teams |
| Fork PR, merge queue, post-merge push | Correct commits, no privileged PR execution, intended checks |

Also exercise two-team requirements with only one team's approval and then both,
and verify stale approvals are handled by the chosen repository settings. A
passing Saga check is not an approval. Report which files changed, which host
settings were actually applied, and which settings still need a maintainer.

References (provider behavior checked 2026-10-18):
- https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow
- https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners
- https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets
- https://docs.github.com/en/rest/repos/rules
- https://docs.gitlab.com/user/project/codeowners/advanced/
