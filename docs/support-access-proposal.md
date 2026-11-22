# Proposed support-access boundary

Status: **proposal for agreement**, not an approved permission grant or an
operational guarantee. This repository's implemented product is a local CLI and
loopback reader; [SECURITY.md](../SECURITY.md) describes that boundary. Its
GitHub Actions release pipeline publishes CLI artifacts, not a customer hosting
environment. Self-hostable-server and self-hosted-onboarding Saga Plans describe
future work. No deployed AWS support roles, operational log-group inventory,
customer-data boundary, grant/revoke automation or customer deployment pipeline
is present in this checkout. Feature branches are not evidence of deployed
infrastructure. No cloud state or permissions were changed for this proposal.

## Three separate grants

| Purpose | Proposed access | Boundary and evidence required before use |
| --- | --- | --- |
| Routine support | Read specified operational CloudWatch log groups/streams and operational metrics | Name account, region, environment, role, log groups, metric visibility and intended operators. Logs access includes the messages. Review producers, retention, existing content and effective IAM grants first. |
| Updates | Trigger only the approved deployment pipeline, only when granted | Name pipeline/environment, permitted artifact/source, release approval and rollback path. Trigger permission does not imply IAM administration, shell/database access or authority to edit the pipeline or its deployment role. |
| Incident investigation | Specific temporary elevation with approval and an audit record | Record incident, purpose, customer/resource scope, exact actions, operator, approving person, grant/revoke owner, start and expiry, and revocation evidence. Do not make this a standing customer-data grant. |

They are independent decisions. Routine support access does not authorize an
update or elevation. Deployment permissions can change software that handles
customer data, so the pipeline's execution role and controls need review as well
as its trigger. Broader audited support access remains an option for discussion;
it has not been approved.

## CloudWatch Logs reads expose contents

Checked AWS's official managed-policy reference on 2026-11-20:
[`CloudWatchLogsReadOnlyAccess`](https://docs.aws.amazon.com/aws-managed-policy/latest/reference/CloudWatchLogsReadOnlyAccess.html)
is default policy v12, edited 2026-02-12. It permits `logs:Get*`,
`logs:FilterLogEvents`, queries and live tail on `Resource: "*"`, among other
actions. This includes log contents, not just names, timestamps or other metadata.
The managed policy is broader than this proposal and can change over time.
Do not attach it as a substitute for scoping the operational resources.

Proposed minimal API access is an explicit allowlist: `logs:DescribeLogStreams`
and `logs:FilterLogEvents` for approved log groups; `logs:GetLogEvents` for their
approved streams. Use exact partition/account/region/group ARNs, with only the
stream suffix wildcarded if all streams in that group are approved. Query and
console discovery features require separately reviewed actions and dependencies;
do not add broad `logs:Get*`/`logs:Describe*` to make the console work. AWS defines
group and stream resource types separately in its
[Logs authorization reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_logs.html).

An illustrative resource mapping for review, **not an applied policy**:

| Actions | Resource template (replace every placeholder after agreement) |
| --- | --- |
| `logs:DescribeLogStreams`, `logs:FilterLogEvents` | `arn:<partition>:logs:<region>:<account>:log-group:<approved-operational-group>` |
| `logs:GetLogEvents` | `arn:<partition>:logs:<region>:<account>:log-group:<approved-operational-group>:log-stream:*` |

Confirm the required log-group ARN form for each API in AWS's
[IAM guide](https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/iam-identity-based-access-control-cwl.html)
and test allowed and denied resources before granting. A tenant filter in a
query is not an access boundary within a shared group. Exclude customer-content
groups, exports, subscriptions, cross-account observability and unmasking unless
separately approved. Resource scoping must account for other identity/resource
policies, assumed roles and cross-account links; adding a narrow allow does not
remove an existing broad grant.

## Metrics are a separate permission surface

Log reads and metric reads are not interchangeable. Proposed operational metric
reads are `cloudwatch:ListMetrics`, `cloudwatch:GetMetricData`, and
`cloudwatch:GetMetricStatistics`, only if needed. Classic metric reads cannot
be scoped to individual metric ARNs using these actions. The current
[CloudWatch authorization reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_cloudwatch.html)
lists no resource type for `GetMetricStatistics` and optional dataset resources
for `GetMetricData` and `ListMetrics`; those dataset resources do not establish
per-metric scoping for classic metrics. Classic reads require `Resource: "*"`;
the `cloudwatch:namespace` condition is not a general namespace restriction for
these reads. A dashboard filter is not an IAM boundary. Limit account/region
where supported and explicitly review wider metric visibility. If unacceptable,
use an isolated operational account or a reviewed mediated metric surface;
do not promise namespace-only reads. Review names, dimensions and labels for
customer identifiers. No metrics policy is granted by this document.

## Data handling and incident workflow

Keep customer content, credentials, request/response bodies, prompts, documents,
query text and database values out of operational logs. Prefer allowlisted
event codes, durations, counts and safe correlation identifiers. Redact before
emission and test representative failure paths and library/framework logging.
Inspect existing log samples and retention before classifying a group as
operational. There is no evidence here that existing deployed logs are already
content-free. If they contain customer content, routine read access is customer
data access and must be excluded or explicitly approved under a revised design.

For an incident, the proposed process is:

1. Open an access request linked to the incident. Specify purpose, data classes,
   resources, actions, operator and duration; use sanitized diagnostics first.
2. Obtain approval from the designated authority for that scope. Record who
   approved what and when before the designated grant owner enables access.
3. Grant a narrowly scoped expiring session. Record session identity, policy,
   actual start and expiry, and the relevant audit destination. Do not put
   customer data or secrets in the approval record.
4. Investigate within that scope. Retain an activity trail and any necessary
   evidence in approved storage with its own retention/access controls.
5. Expire/revoke the grant and verify effective access is removed, including
   active sessions as applicable. Record completion, reviewer and any retained
   evidence. Escalate failed revocation to the named owner.

Audit coverage is an implementation requirement, not something this proposal
proves. Select the identity, IAM/session and service event sources, including
any required data-event configuration, retention and review owner. Test that
the intended log/data reads, deployment triggers and elevation lifecycle can be
traced; do not assume an audit system captures every read by default.

## Decisions still required

| Decision | Current state |
| --- | --- |
| Who approves routine operational access and its scope? | Unassigned; customer/organization agreement required. |
| Who authorizes each update and which pipeline/environment? | Unassigned; no customer deployment pipeline identified here. |
| Who approves customer-data elevation, including emergencies? | Unassigned; do not assume a support operator may self-approve. |
| Who grants access, enforces expiry, revokes and verifies removal? | Unassigned; decide primary and backup owners and timing. |
| Which accounts, groups, metrics, customers and data classes are authorized? | Unspecified; inventory and classification required. |
| Who owns audit retention, review and incident closure? | Unassigned; event coverage and retention need verification. |
| Is broader audited support access acceptable? | Undecided; requires an explicit agreement and revised scope. |

Proposed customer-facing wording, contingent on the implemented design and
agreement:

> We have no standing access to customer data; additional access requires approval and is audited.

Use this only after verifying effective permissions, log/metric contents,
deployment boundaries, temporary access and audit coverage support the statement.
It is not an established promise about current operations. Any broader standing
access agreement requires different wording. Approval of this proposal must name
the unresolved owners and exact scope; this document does not authorize grants,
revocations, deployments or external communications.
