#!/usr/bin/env python3
"""Author the complete native DayLight model from exact, read-only source pins.

Python standard library only. Never publishes, follows heads, or reads source
SVGs. Layout is an authored neighborhood grid with explicit dogleg lane recipes,
not an obstacle-search router. Output must be outside the input saga.
"""
import argparse
from collections import Counter, defaultdict
import hashlib
import json
import math
from pathlib import Path
import xml.etree.ElementTree as ET

# Migration provenance and exact entity directories, not rendered dependencies.
PINS = [('local-cloud-overview', 'r2', 'The system across its stores'),
 ('local-capture-delivery', 'r5-viewer-safe', 'Capture & delivery'),
 ('cloud-source-lineage', 'r5', 'Source admission'),
 ('cloud-corpus', 'r4', 'Cloud corpus'),
 ('local-project-boundary', 'r5-viewer-safe', 'Project scope'),
 ('local-index-progress', 'r5-viewer-safe', 'Index progress'),
 ('local-search-retrieval', 'r5-viewer-safe', 'Local search'),
 ('local-episode-catalog', 'r5-viewer-safe', 'Episode catalog'),
 ('local-episode-correlation', 'r5-viewer-safe', 'Episode links'),
 ('local-whole-model', 'r5-viewer-safe', 'Whole local model'),
 ('cloud-project-authority', 'r5', 'Project authority'),
 ('cloud-identity', 'r4', 'Identity & tenancy'),
 ('cloud-contribution', 'r4', 'Sharing rules'),
 ('cloud-shared-destinations', 'r4', 'Projects & Teams'),
 ('cloud-oauth', 'r4', 'MCP grants'),
 ('cloud-invitations', 'r4', 'Invitations'),
 ('cloud-enterprise', 'r4', 'Enterprise'),
 ('cloud-scim', 'r4', 'SCIM directory'),
 ('cloud-messaging', 'r5', 'Durable messages'),
 ('cloud-overview', 'r3', 'Whole cloud model')]

CORE_SUMMARIES = {'local-capture-observation-offsets': 'Observed byte progress, independent of cloud receipts.',
 'local-project-source-grants': 'Exact source generation and accepted storage policy.',
 'local-pending-prompt-payloads': 'One contained observation shared by delivery rows.',
 'local-pending-project-deliveries': 'Retry state for one destination and source generation.',
 'cloud-project-sources': 'Admitted source and its accepted policy revision.',
 'cloud-project-delivery-admissions': 'Records the source grant and generation of a delivery.',
 'cloud-prompt': 'Project-scoped identity, source pointer and search.',
 'local-local-projects': 'Durable local choices; hosted binding is optional.',
 'local-local-project-file-memberships': 'Rebuildable Project-to-file query scope; no SQL FK.',
 'local-source-files': 'Private file identity and indexed byte progress.',
 'local-search-documents': 'Searchable record identity and source byte range.',
 'local-episodes': 'Rebuildable catalog; manifests remain authoritative.',
 'local-episode-correlation-targets': 'Search-side episode revision and timing projection.',
 'local-episode-artifacts': 'Artifact path, size and hash; files retain authority.',
 'local-episode-evidence-links': 'Versioned episode evidence with confidence and timing.',
 'cloud-projects': 'Private capture scope; requires a direct person Owner.',
 'cloud-project-policy-versions': 'Immutable policies; Project selects current revision.',
 'cloud-teams': 'Shared Project destinations, distinct from people Teams.',
 'cloud-shared-project-sources': 'Consent for an exact source generation and rule revision.',
 'cloud-team-members': 'Shared Project membership and current evidence.',
 'cloud-team-source-rule-versions': 'Immutable sharing rules; consent pins a revision.',
 'cloud-users': 'A person identified by durable login issuer and subject.',
 'cloud-organizations': 'Organization authority and lifecycle for people Teams.'}
CORE_LABELS = {'local-capture-observation-offsets': 'Observation progress',
 'local-project-source-grants': 'Source grant / generation',
 'local-pending-prompt-payloads': 'Captured payload',
 'local-pending-project-deliveries': 'Destination delivery debt',
 'cloud-project-sources': 'Admitted source / generation',
 'cloud-project-delivery-admissions': 'Admitted delivery',
 'cloud-prompt': 'Hosted search projection',
 'local-local-projects': 'Durable local Project',
 'local-local-project-file-memberships': 'Projected query membership',
 'local-source-files': 'Indexed source file',
 'local-search-documents': 'Retrievable document',
 'local-episodes': 'Manifest-backed episode',
 'local-episode-correlation-targets': 'Episode target projection',
 'local-episode-artifacts': 'Catalog artifact',
 'local-episode-evidence-links': 'Versioned evidence link',
 'cloud-projects': 'Private storage Project',
 'cloud-project-policy-versions': 'Immutable policy revision',
 'cloud-teams': 'Shared Project (teams)',
 'cloud-shared-project-sources': 'Exact-source consent',
 'cloud-team-members': 'Shared Project membership',
 'cloud-team-source-rule-versions': 'Accepted sharing rules',
 'cloud-users': 'Principal',
 'cloud-organizations': 'Organization'}
CORE_CONNECTION_LABELS = {('local-pending-prompt-payloads', 'observed-file'): 'application transaction',
 ('local-pending-project-deliveries', 'fk-observationid'): 'FK / observationId',
 ('local-pending-project-deliveries', 'admitting-grant'): 'copied authority '
                                                          'tuple',
 ('cloud-project-delivery-admissions', 'fk-2'): 'FK / source_grant_id',
 ('cloud-prompt', 'logical-admission'): 'application join / delivery_id',
 ('cloud-project-sources', 'fk-1'): 'FK / storage Project',
 ('cloud-projects', 'fk-2'): 'FK / current policy',
 ('cloud-shared-project-sources', 'fk-7'): 'FK / exact source lineage',
 ('cloud-shared-project-sources', 'fk-2'): 'FK / storage Project',
 ('cloud-shared-project-sources', 'fk-1'): 'FK / shared destination',
 ('cloud-shared-project-sources', 'fk-6'): 'FK / accepted rule',
 ('cloud-team-members', 'fk-1'): 'FK / membership',
 ('cloud-team-members', 'fk-2'): 'FK / principal',
 ('cloud-teams', 'fk-1'): 'FK / Organization',
 ('local-local-project-file-memberships', 'project-projection'): 'application '
                                                                 '/ durable '
                                                                 'choices',
 ('local-local-project-file-memberships', 'member-file'): 'application / file '
                                                          'identity',
 ('local-search-documents', 'fk-file-id'): 'FK / file_id',
 ('local-episode-correlation-targets', 'manifest-projection'): 'projection / '
                                                               'manifest '
                                                               'revision',
 ('local-episode-artifacts', 'fk-episode-id'): 'FK / episode_id',
 ('local-episode-evidence-links', 'episode-edge'): 'application / episode '
                                                   'identity',
 ('local-episode-evidence-links', 'fk-document-rowid'): 'FK / document_rowid'}
CORE_RELATIONSHIPS = [('local-pending-prompt-payloads', 'observed-file'),
 ('local-pending-project-deliveries', 'fk-observationid'),
 ('local-pending-project-deliveries', 'admitting-grant'),
 ('cloud-project-delivery-admissions', 'fk-2'),
 ('cloud-prompt', 'logical-admission'),
 ('cloud-project-sources', 'fk-1'),
 ('cloud-projects', 'fk-2'),
 ('cloud-shared-project-sources', 'fk-7'),
 ('cloud-shared-project-sources', 'fk-2'),
 ('cloud-shared-project-sources', 'fk-1'),
 ('cloud-shared-project-sources', 'fk-6'),
 ('cloud-team-members', 'fk-1'),
 ('cloud-team-members', 'fk-2'),
 ('cloud-teams', 'fk-1'),
 ('local-local-project-file-memberships', 'project-projection'),
 ('local-local-project-file-memberships', 'member-file'),
 ('local-search-documents', 'fk-file-id'),
 ('local-episode-correlation-targets', 'manifest-projection'),
 ('local-episode-artifacts', 'fk-episode-id'),
 ('local-episode-evidence-links', 'episode-edge'),
 ('local-episode-evidence-links', 'fk-document-rowid')]

def file_digest(path):
    return 'sha256:' + hashlib.sha256(path.read_bytes()).hexdigest()


def overlaps(a, b):
    x, y, w, h = a
    u, v, c, d = b
    return not (x+w <= u or u+c <= x or y+h <= v or v+d <= y)


def center(rect):
    x, y, width, height = rect
    return x+width/2, y+height/2


def boundary(rect, toward):
    """The shared renderer clips a connection ray to its endpoint rectangle."""
    x, y = center(rect)
    dx, dy = toward[0]-x, toward[1]-y
    if dx == 0 and dy == 0:
        return x, y
    scale = min(rect[2]/2/abs(dx) if dx else float('inf'),
                rect[3]/2/abs(dy) if dy else float('inf'))
    return x+dx*scale, y+dy*scale


def segment_hits(a, b, rect):
    """Open-interior segment/rectangle check; touching a border is allowed."""
    x, y, width, height = rect
    x, y, width, height = x+1, y+1, width-2, height-2
    low, high = 0., 1.
    dx, dy = b[0]-a[0], b[1]-a[1]
    for p, q in [(-dx,a[0]-x),(dx,x+width-a[0]),(-dy,a[1]-y),(dy,y+height-a[1])]:
        if p == 0:
            if q < 0:
                return False
        elif p < 0:
            low = max(low, q/p)
        else:
            high = min(high, q/p)
    return low < high


# Each neighborhood occupies two authored columns and consecutive rows. These
# are semantic homes, not copies of the original ERD document boundaries.
# key, parent, label, provenance, column, first row, ordered entity keys
NEIGHBORHOODS = [
    ('capture', 'delivery-area', 'Observe bytes, admit a generation, retire delivery debt', 'local-capture-delivery', 0, 0,
     'local-capture-observation-offsets local-project-source-grants local-pending-prompt-payloads local-pending-project-deliveries local-project-source-high-waters local-agent-kv'),
    ('hosted', 'delivery-area', 'Exact source identity and admitted delivery', 'cloud-source-lineage', 2, 0,
     'cloud-project-sources cloud-project-delivery-admissions cloud-project-source-operations'),
    ('corpus', 'delivery-area', 'Source bytes produce a searchable hosted corpus', 'cloud-corpus', 4, 0,
     'cloud-prompt cloud-corpus-migration-scans cloud-corpus-object-migrations'),
    ('local-scope', 'search-area', 'Durable choices and private exposure review', 'local-project-boundary', 0, 3,
     'local-local-projects local-local-project-sources local-secret-exposures'),
    ('retrieval', 'search-area', 'File identity becomes retrievable text and query scope', 'local-search-retrieval', 0, 5,
     'local-source-files local-search-documents local-local-project-file-memberships local-search-fts local-local-search-meta'),
    ('index-progress', 'search-area', 'Bounded indexing work, checkpoints and diagnostics', 'local-index-progress', 0, 8,
     'local-index-runs local-index-run-sources local-index-run-diagnostics'),
    ('discovery', 'search-area', 'Descriptive folder discovery is not authority', 'local-search-retrieval', 0, 10,
     'local-local-folder-projection local-local-folder-sources'),
    ('episodes', 'search-area', 'Validated manifests remain the recording authority', 'local-episode-catalog', 0, 11,
     'local-episodes local-episode-artifacts local-local-folders local-episode-catalog-meta'),
    ('correlation', 'search-area', 'Versioned episode evidence and resumable correlation', 'local-episode-correlation', 0, 13,
     'local-episode-correlation-targets local-episode-evidence-links local-episode-prompt-bindings local-correlation-jobs'),
    ('storage', 'authority-area', 'Private storage ownership, policy and direct grants', 'cloud-project-authority', 2, 3,
     'cloud-projects cloud-project-policy-versions cloud-project-user-access cloud-project-team-access cloud-project-access-requests'),
    ('sharing', 'authority-area', 'Membership and consent are separate access checks', 'cloud-shared-destinations', 2, 6,
     'cloud-teams cloud-shared-project-sources cloud-team-members cloud-team-source-rule-versions cloud-team-project-contributions cloud-shared-project-legacy-prompts cloud-people-teams cloud-people-team-members cloud-shared-project-teams cloud-shared-project-team-grants'),
    ('directory', 'authority-area', 'Principal and Organization evidence', 'cloud-identity', 2, 11,
     'cloud-users cloud-organizations cloud-organization-members cloud-organization-refresh-sessions cloud-principal-verified-emails'),
    ('oauth', 'authority-area', 'Connector consent produces narrowly scoped tokens', 'cloud-oauth', 4, 3,
     'cloud-mcp-oauth-clients cloud-mcp-oauth-requests cloud-mcp-oauth-codes cloud-mcp-oauth-refresh-tokens cloud-mcp-oauth-access-bindings'),
    ('invitations', 'authority-area', 'Restricted links become evidenced membership', 'cloud-invitations', 4, 6,
     'cloud-team-invitation-links cloud-team-invitation-link-audiences cloud-team-invitation-link-acceptances cloud-team-invitation-link-events cloud-team-invitation-link-operations cloud-team-invitation-browser-intents cloud-team-invitation-handoffs cloud-team-bootstrap-receipts'),
    ('enterprise', 'authority-area', 'Managed identity mappings and administrative evidence', 'cloud-enterprise', 4, 10,
     'cloud-managed-organization-directory cloud-managed-team-groups cloud-group-org-role-mapping cloud-group-mapping-seed-state cloud-audit-event'),
    ('scim', 'authority-area', 'Provisioned directory resources are distinct from login', 'cloud-scim', 4, 13,
     'cloud-scim-users cloud-scim-groups cloud-scim-group-members cloud-scim-provisioning-tokens'),
    ('messaging', 'authority-area', 'Durable messages and recipient inbox queries', 'cloud-messaging', 4, 15,
     'cloud-message'),
]

SUPPORT_SUMMARIES = {
    'local-project-source-high-waters': 'Per-generation, per-file admission boundary; distinct from observed progress and ACK.',
    'local-agent-kv': 'Durable agent settings, continuity data and capture schema marker.',
    'cloud-project-source-operations': 'Idempotent source admission and lifecycle operations, bound to the produced generation.',
    'cloud-team-project-contributions': 'Legacy storage-to-shared Project contribution; activation sets a visibility floor.',
    'cloud-corpus-migration-scans': 'Resumable corpus scan cursors; accepted sends advance scan progress.',
    'cloud-corpus-object-migrations': 'Per-object conversion outcomes, attempts, verified destination and blocked error.',
    'local-local-project-sources': 'Path-free local source membership; Project deletion cascades.',
    'local-secret-exposures': 'Private exposure review; dismissal retains a tombstone to suppress repeats.',
    'local-index-runs': 'Bounded indexing progress, omissions, failures, retry and pause state.',
    'local-index-run-sources': 'Per-run file checkpoints; only the run has a physical FK.',
    'local-index-run-diagnostics': 'Per-run counters distinguish omissions from systemic failures.',
    'local-search-fts': 'FTS5 terms align to document rowids through application writes, without a SQL FK.',
    'local-local-folder-projection': 'Private folder discovery metadata, separate from durable Project choices.',
    'local-local-folder-sources': 'Discovered folder membership; file identity is an application association.',
    'local-local-folders': 'Catalog folder snapshot describes recordings; it grants no recording or cloud consent.',
    'local-episode-catalog-meta': 'Episode catalog schema marker; a mismatch makes storage unavailable.',
    'local-episode-prompt-bindings': 'Current episode-to-prompt resolution; pending or ambiguous sessions can remain null.',
    'local-correlation-jobs': 'Resumable source or episode work; kind determines the meaning of target_id.',
    'local-local-search-meta': 'Search schema and cache metadata in an independently rebuildable store.',
    'cloud-project-user-access': 'Direct person grants on a Project and their revocation history.',
    'cloud-project-team-access': 'Legacy reader grant from a storage Project to a shared Project destination.',
    'cloud-project-access-requests': 'Requests to read a Project and their recorded decisions.',
    'cloud-principal-verified-emails': 'Verified principal email evidence that invitation audience rules may match.',
    'cloud-organization-members': 'Organization membership and the evidence that keeps it current.',
    'cloud-organization-refresh-sessions': 'First-party refresh-token families for a principal, not Organization data.',
    'cloud-shared-project-legacy-prompts': 'Already-visible legacy prompts without admissions; do not infer source lineage.',
    'cloud-people-teams': 'People grouping in one Organization; distinct from shared Projects stored as teams.',
    'cloud-people-team-members': 'Current admin/member participation in a people Team, retaining revocation.',
    'cloud-shared-project-teams': 'Links a shared Project and people Team; a trigger enforces Organization equality.',
    'cloud-shared-project-team-grants': 'Revocable people-Team grant; a composite FK requires an existing shared Project link.',
    'cloud-mcp-oauth-clients': 'Registered MCP OAuth clients and their validated redirect set.',
    'cloud-mcp-oauth-requests': 'Short-lived authorization requests carry no grant scope until consent.',
    'cloud-mcp-oauth-codes': 'Hashed, expiring authorization codes scoped to exactly one Team or Project.',
    'cloud-mcp-oauth-refresh-tokens': 'Hashed refresh tokens scoped to exactly one Team or Project.',
    'cloud-mcp-oauth-access-bindings': 'Access-token hash to grant-scope binding used by connector reads.',
    'cloud-team-invitation-links': 'Restricted invitation link for one Team, retaining only a handle digest.',
    'cloud-team-invitation-link-audiences': 'Exact-email admission rules and the principal who claimed each rule.',
    'cloud-team-invitation-link-acceptances': 'Accepted invitations and the link generation that admitted membership.',
    'cloud-team-invitation-link-events': 'Retention-bounded link lifecycle events; correlation is stored as a digest.',
    'cloud-team-invitation-link-operations': 'Idempotency for invitation creation, revocation and rotation.',
    'cloud-team-invitation-browser-intents': 'Short-lived browser intent resolves a link without exposing its handle.',
    'cloud-team-invitation-handoffs': 'One-time code carries a resolved browser intent back to the native app.',
    'cloud-team-bootstrap-receipts': 'Idempotent bootstrap result: Team, owner membership and optional invitation.',
    'cloud-managed-organization-directory': 'Managed Organization issuer, username claim and group mapping source.',
    'cloud-managed-team-groups': 'Provider groups that grant a role in a managed Team.',
    'cloud-group-org-role-mapping': 'Enterprise provider group to Organization role mapping.',
    'cloud-group-mapping-seed-state': 'Applied mapping seed record; its forensic hash never triggers reseeding.',
    'cloud-audit-event': 'Administrative evidence for identity, mapping and authorization changes.',
    'cloud-scim-users': 'Provisioned SCIM users, separate from product login principals.',
    'cloud-scim-groups': 'Enterprise SCIM group resources.',
    'cloud-scim-group-members': 'Membership between provisioned SCIM users and groups.',
    'cloud-scim-provisioning-tokens': 'Hashed bearer credentials and lifecycle metadata for SCIM provisioning.',
    'cloud-message': 'Durable person-to-person messages; inboxes query the recipient.',
}

MARKERS = {'0..many': 'zero-or-many', '0..1': 'zero-or-one', '1': 'only-one'}
INITIAL_CAMERA = dict(x=0, y=0, width=9300, height=7200)
COLUMN_PITCH, CARD_WIDTH = 4400, 2700


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('saga', type=Path, help='read-only DayLight saga copy')
    parser.add_argument('output', type=Path, help='new directory outside the saga')
    parser.add_argument('--rendered-svg', type=Path, help='shared-renderer SVG for diagnostic previews')
    args = parser.parse_args()
    root, output = args.saga.resolve(), args.output.resolve()
    if output == root or root in output.parents:
        parser.error('output must be outside the source saga')

    source_reads, records, entities, appearances, manifest = {}, {}, {}, defaultdict(list), []

    def read(path):
        resolved = path.resolve()
        if root not in resolved.parents:
            raise ValueError('source escapes saga: '+str(path))
        source_reads[path] = file_digest(path)
        return json.loads(path.read_text())

    for key, revision, label in PINS:
        path = root/'___inventory'/'erds'/(key+'.erd')/'revisions'/(revision+'.json')
        record = read(path)
        assert record['id'] == revision
        records[key] = record
        manifest.append(dict(id=key, label=label, target=record['record'],
                             revision=record['record']+':revision:'+revision,
                             revision_digest=source_reads[path], explanation=record['explanation'],
                             role='exact directory and narrative provenance; no imported visual'))
        for pin in record['directory']:
            k, rev = pin['target'].split(':')[-1], pin['revision'].split(':')[-1]
            entity = read(root/'___inventory'/'data-entities'/(k+'.data-entity')/'revisions'/(rev+'.json'))
            assert entity['record'] == pin['target'] and entity['id'] == rev
            if k in entities:
                assert entities[k] == entity, 'conflicting exact entity revisions: '+k
            entities[k] = entity
            appearances[k].append(key)

    def ref(k):
        r = entities.get(k, records.get(k))
        return dict(target=r['record'], revision=r['record']+':revision:'+r['id'])

    summaries = {**CORE_SUMMARIES, **SUPPORT_SUMMARIES}
    homes, slots = {}, {}
    for key, parent, label, provenance, col, row, names in NEIGHBORHOODS:
        for i, entity in enumerate(names.split()):
            assert entity not in homes, entity
            homes[entity] = key
            slots[entity] = (col+i % 2, row+i//2)
    assert set(homes) == set(entities) == set(summaries)
    assert len(entities) == 76 and sum(len(e['fields']) for e in entities.values()) == 675

    # Preserve record-owner direction, including fk-policy-parent whose source
    # explicitly documents a SQL child direction opposite to the record owner.
    connections = []
    for owner in homes:
        for relationship in entities[owner].get('relationships', []):
            destination = relationship['destination']['target'].split(':')[-1]
            assert destination in entities and ref(destination) == relationship['destination']
            cardinality = relationship['cardinality']
            if set(cardinality) != {'owner', 'destination'} or any(v not in MARKERS for v in cardinality.values()):
                raise ValueError(f'unknown cardinality: {owner}/{relationship["id"]}: {cardinality}')
            connections.append(dict(id=owner+'--'+relationship['id'], owner=owner, destination=destination,
                                    relation=relationship, source={**ref(owner), 'relationship': relationship['id']},
                                    kind='fk' if relationship['explanation'].lower().startswith('physical') else 'projection',
                                    label=CORE_CONNECTION_LABELS.get((owner, relationship['id']), relationship['label']), support=relationship['explanation'],
                                    core=(owner, relationship['id']) in CORE_RELATIONSHIPS))
    assert len(connections) == 120
    for key, owner, destination, label, kind, provenance in [
        ('same-source-generation', 'local-project-source-grants', 'cloud-project-sources', 'Same grant / generation', 'projection', 'local-cloud-overview'),
        ('delivery-admission', 'local-pending-project-deliveries', 'cloud-project-delivery-admissions', 'Server admission', 'transport', 'local-cloud-overview'),
        ('read-authorization', 'cloud-shared-project-sources', 'cloud-prompt', 'Membership + exact-source consent', 'authorization', 'local-cloud-overview'),
        ('storage-binding', 'local-local-projects', 'cloud-projects', 'Optional hosted storage binding', 'projection', 'local-cloud-overview'),
    ]:
        connections.append(dict(id=key, owner=owner, destination=destination, relation=None,
                                source=ref(provenance), kind=kind, label=label,
                                support=records[provenance]['explanation'], core=True))
    # Immutable source bytes are explained in the corpus section's prose. No
    # invented table or multiplicity is needed for that transport explanation.
    incident, outgoing, row_ends = defaultdict(list), defaultdict(list), defaultdict(list)
    for c in connections:
        outgoing[c['owner']].append(c['id'])
        for side, key in [('from', c['owner']), ('to', c['destination'])]:
            incident[key].append((c['id'], side))
            row_ends[slots[key][1]].append((c['id'], side))
    heights = {k: max(640, 211+28*len(e['fields']), 100+math.ceil(len(incident[k])/2)*125)
               for k, e in entities.items()}
    row_top, row_track, row_bottom = {}, {}, {}
    cursor = 2400
    for row in range(16):
        keys = [k for k in homes if slots[k][1] == row]
        row_top[row] = cursor
        occupied = max((heights[k]+180+320*len(outgoing[k]) for k in keys), default=900)
        row_track[row] = cursor+occupied+180
        row_bottom[row] = row_track[row]+24*len(row_ends[row])+400
        cursor = row_bottom[row]+400
    width, height = 27000, row_bottom[15]+500
    assert height < 100000

    def style(fill='none', stroke='none', ink='@diagram-normal-ink', font=100, width=3):
        return dict(fill=fill, stroke=stroke, ink=ink, font_size=font, stroke_width=width)

    board = dict(version=1, width=width, height=height, viewport=INITIAL_CAMERA,
                 background='@diagram-canvas', styles={
        'hero': style(font=180), 'body': style(font=120), 'small': style(font=100), 'area': style(font=100),
        'entity': style('@diagram-normal-fill', '@diagram-normal-stroke', font=140, width=4),
        'fk': style(stroke='@diagram-normal-stroke', font=112, width=5),
        'projection': dict(**style(stroke='@diagram-blue-accent', font=112, width=5), dash=True),
        'transport': style(stroke='@diagram-yellow-accent', font=112, width=7),
        'authorization': dict(**style(stroke='@diagram-purple-accent', font=112, width=6), dash=True),
    }, elements=[])
    by_id, paths, bounds, bodies, labels = {}, {}, {}, {}, {}

    def box(rect):
        return dict(zip(('x', 'y', 'width', 'height'), rect))

    def add(e, absolute=None):
        key = e['id']
        assert key not in by_id
        if absolute is not None:
            bounds[key] = absolute
            x, y, w, h = absolute
            if e.get('parent'):
                px, py, *_ = bounds[e['parent']]
                x, y = x-px, y-py
            e.update(box((x, y, w, h)))
        by_id[key] = e
        paths[key] = (paths[e['parent']]+'/' if e.get('parent') else '')+key
        board['elements'].append(e)
        return e

    def section(key, label, rect, provenance, parent=None):
        color = {'delivery-area': 'yellow', 'search-area': 'blue', 'authority-area': 'purple'}.get(parent or key, 'gray')
        e = dict(id=key, kind='group', shape='section', label=label, style='area', color=color, source=ref(provenance))
        if parent:
            e['parent'] = parent
        add(e, rect)

    section('context-area', 'One system / independent stores, explicit authority', (100, 100, 26400, 1450), 'local-cloud-overview')
    section('delivery-area', '01 / Capture, source admission and derived cloud corpus',
            (100, 1650, 26400, row_bottom[2]-1450), 'local-capture-delivery')
    section('search-area', '02 / Local search and episode evidence',
            (100, row_top[3]-500, 8800, row_bottom[14]-row_top[3]+700), 'local-search-retrieval')
    section('authority-area', '03 / Storage ownership, membership, consent and identity',
            (9000, row_top[3]-500, 17500, row_bottom[15]-row_top[3]+700), 'cloud-project-authority')
    for key, parent, label, provenance, col, row, names in NEIGHBORHOODS:
        last_row = row+(len(names.split())-1)//2
        section(key, label, (col*COLUMN_PITCH+200, row_top[row]-250, 8500,
                             row_bottom[last_row]-row_top[row]+350), provenance, parent)

    for key, text, rect, styling, provenance in [
        ('title', 'DayLight / one connected data model', (350, 300, 24500, 260), 'hero', 'local-cloud-overview'),
        ('intro', 'Start with capture: observed progress, source consent and delivery debt are different state. Follow admission into the cloud; continue below for local search and access authority.',
         (350, 660, 8200, 520), 'body', 'local-cloud-overview'),
        ('coverage', '76 native tables / 675 fields / 120 recorded relationships. Select a table or line for exact source evidence; Details reveals all supporting links and fields.',
         (350, 1180, 8200, 300), 'small', 'cloud-overview'),
        ('legend', 'Crow’s feet show recorded cardinality. Solid gray = physical FK; dashed blue = application association. Yellow = transport; purple = authorization. These colors do not imply identifying relationships.',
         (10000, 660, 7900, 520), 'body', 'local-cloud-overview'),
        ('source-bytes-story', 'Immutable source bytes remain in object storage. PostgreSQL holds scope, identity and derived search state; delivery admission records exact lineage.',
         (18500, 660, 7500, 520), 'body', 'cloud-corpus'),
    ]:
        add(dict(id=key, kind='text', label=text, style=styling, wrap=True, source=ref(provenance)), rect)
        bodies[key] = rect

    for key in homes:
        e = entities[key]
        fields = []
        for field in e['fields']:
            f = dict(name=field['name'], type=field['type'])
            keys = ','.join(short for long, short in [('primary', 'pk'), ('foreign', 'fk')] if long in field.get('keys', []))
            if keys:
                f['key'] = keys
            fields.append(f)
        col, row = slots[key]
        rect = (700+col*COLUMN_PITCH, row_top[row], CARD_WIDTH, heights[key])
        # A source table's actual name remains visible; the summary explains it.
        add(dict(id=key, kind='node', shape='entity', label=CORE_LABELS.get(key, e['name'].replace('_', ' ')),
                 parent=homes[key], style='entity', fields=fields, summary=summaries[key],
                 source=ref(key), description=e['explanation'],
                 note='**'+e['name']+'**\n\n'+summaries[key]), rect)
        bodies[key] = rect

    # Every connection has its own vertical column-gutter lane and row track.
    # Entity-side lanes alternate left/right and spread terminators along the
    # boundary. These fixed recipes never search for paths or join into a bus.
    def endpoint(key, connection_id, end):
        index = incident[key].index((connection_id, end))
        sign = -1 if index % 2 == 0 else 1
        slot, count = index//2, math.ceil((len(incident[key])-(index % 2))/2)
        x, y, w, h = bounds[key]
        offset = (slot-(count-1)/2)*125
        distance = 200+slot*18
        exit_x = x-distance if sign < 0 else x+w+distance
        # Account for ray clipping so endpoint separation is exactly 125 units.
        exit_y = y+h/2+offset*(w/2+distance)/(w/2)
        row = slots[key][1]
        track_y = row_track[row]+24*row_ends[row].index((connection_id, end))
        return (exit_x, exit_y), (exit_x, track_y)

    evidence, lane_counts = [], Counter()
    for index, c in enumerate(connections):
        owner, destination = c['owner'], c['destination']
        start, low_start = endpoint(owner, c['id'], 'from')
        end, low_end = endpoint(destination, c['id'], 'to')
        corridor = max(slots[owner][0], slots[destination][0])
        lane = corridor*COLUMN_PITCH+3600+18*lane_counts[corridor]
        lane_counts[corridor] += 1
        assert lane < (corridor+1)*COLUMN_PITCH+650, ('lane capacity', corridor)
        bends = [start, low_start, (lane, low_start[1]), (lane, low_end[1]), low_end, end]
        x, y, w, h = bounds[owner]
        label = (x, y+h+180+320*outgoing[owner].index(c['id']), w, 300)
        relation = c['relation']
        e = dict(id=c['id'], kind='connection', x=0, y=0, style=c['kind'],
                 **{'from': paths[owner], 'to': paths[destination]},
                 bends=[dict(x=a, y=b) for a, b in bends], label=c['label'], label_box=box(label),
                 wrap=True, head_size=100, head='arrow', source=c['source'],
                 description=c['support'], note=c['support'][:900].replace('<', r'\<'))
        if relation:
            e.update(tail=MARKERS[relation['cardinality']['owner']], head=MARKERS[relation['cardinality']['destination']])
        if not c['core']:
            e['visibility'] = 'detail'
        add(e)
        labels[c['id']] = label
        evidence.append(dict(id=c['id'], kind={'fk': 'physical_fk', 'projection': 'application_projection',
                                              'transport': 'transport_explanation', 'authorization': 'authorization_explanation'}[c['kind']],
                             recorded=relation is not None, source=c['source'],
                             **{'from': paths[owner], 'to': paths[destination]},
                             supporting_text=c['support'], code=relation.get('code', []) if relation else [],
                             endpoint_pins=[ref(owner), ref(destination)], endpoint_geometry='authored native entity bounds',
                             cardinality=relation['cardinality'] if relation else None,
                             markers={k: e.get(k, 'none') for k in ('tail', 'head')},
                             visibility=e.get('visibility', 'always')))

    # Coverage is a set equality, not an inferred count or imported-image claim.
    expected = {(k, r['id']) for k, e in entities.items() for r in e.get('relationships', [])}
    actual = {(e['source']['target'].split(':')[-1], e['source']['relationship'])
              for e in evidence if e['recorded']}
    assert actual == expected and len(actual) == 120
    assert len(board['elements']) < 500
    for key, rect in {**bodies, **labels}.items():
        x, y, w, h = rect
        assert min(x, y) >= 0 and x+w <= width and y+h <= height, ('outside canvas', key)
    for i, (key, rect) in enumerate(bodies.items()):
        for other, other_rect in list(bodies.items())[i+1:]:
            assert not overlaps(rect, other_rect), ('body overlap', key, other)
    for key, rect in labels.items():
        for other, other_rect in {**bodies, **labels}.items():
            if key != other:
                assert not overlaps(rect, other_rect), ('label overlap', key, other)
    for e in board['elements']:
        if e['kind'] != 'connection':
            continue
        a_key, b_key = (e[k].split('/')[-1] for k in ('from', 'to'))
        bends = [(p['x'], p['y']) for p in e['bends']]
        points = [boundary(bounds[a_key], bends[0])]+bends+[boundary(bounds[b_key], bends[-1])]
        for key, rect in {**bodies, **labels}.items():
            if key in (a_key, b_key):
                continue
            assert not any(segment_hits(a, b, rect) for a, b in zip(points, points[1:])), ('route collision', e['id'], key)
        # A conservative envelope includes the complete 100-unit marker even
        # when the short endpoint ray is angled. Contact with its own table is
        # intentional; a neighboring body or label must remain clear.
        for endpoint_key, point in [(a_key, points[0]), (b_key, points[-1])]:
            marker_box = (point[0]-150, point[1]-150, 300, 300)
            for key, rect in {**bodies, **labels}.items():
                if key != endpoint_key:
                    assert not overlaps(marker_box, rect), ('marker collision', e['id'], key)
    for key in homes:
        x, y, w, h = bounds[key]
        px, py, pw, ph = bounds[homes[key]]
        assert px <= x and py+150 <= y and x+w <= px+pw and y+h <= py+ph, ('section bounds', key)
    for path, digest in source_reads.items():
        assert file_digest(path) == digest, ('source changed', path)

    recorded_incident = {k for c in connections if c['relation'] for k in (c['owner'], c['destination'])}
    audit = dict(unique_pins=len(manifest), frame_instances=0, primary_native_entities=len(entities),
                 core_narrative_entities=len(CORE_SUMMARIES), covered_entities=len(entities), native_fields=675,
                 recorded_relationships=len(actual), physical_relationships=sum(e['kind']=='physical_fk' for e in evidence),
                 application_relationships=sum(e['recorded'] and e['kind']=='application_projection' for e in evidence),
                 unknown_cardinalities=[], connections=len(evidence), source_modified=False, published=False,
                 source_svg_required=False, verified_source_files=len(source_reads),
                 source_pins='source-pins.json', connection_evidence='connection-evidence.json', entity_evidence='entity-evidence.json',
                 disconnected_entities=sorted(set(entities)-recorded_incident),
                 geometry_checks=['body separation', 'label/body separation', 'label/label separation', 'route/body clearance',
                                  'route/label clearance', 'native section gutters', 'marker/body and marker/label clearance'],
                 source_limit='Exact committed-code documentation; cardinalities and enforcement are source declarations, not live database inspection.',
                 detail_policy='All 76 tables and 675 fields are native. Core 21 recorded links and 4 narrative explanations are always present; 99 supporting links reveal in Details.',
                 entity_coverage=[dict(**ref(k), primary_instance=paths[k], neighborhood=homes[k], source_appearances=appearances[k]) for k in homes],
                 sections=[dict(id=k, label=by_id[k]['label'], **box(bounds[k])) for k in ['context-area', 'delivery-area', 'search-area', 'authority-area']])
    entity_evidence = [dict(source=ref(k), record=entities[k], source_appearances=appearances[k],
                           revision_digest=source_reads[root/'___inventory'/'data-entities'/(k+'.data-entity')/
                                                        'revisions'/(entities[k]['id']+'.json')]) for k in homes]
    camera = dict(initial=INITIAL_CAMERA, canvas=dict(width=width, height=height),
                  tours=[dict(name=k, anchor=k, **box(bounds[k])) for k in ['delivery-area', 'search-area', 'authority-area']+[n[0] for n in NEIGHBORHOODS]])
    output.mkdir(parents=True, exist_ok=True)
    for name, data in [('daylight-board.json', board), ('source-pins.json', manifest), ('source-audit.json', audit),
                       ('connection-evidence.json', evidence), ('entity-evidence.json', entity_evidence), ('camera-proposal.json', camera)]:
        (output/name).write_text(json.dumps(data, indent=2)+'\n')
    if args.rendered_svg:
        rendered = args.rendered_svg.read_text()
        expected = f'0 0 {width} {height}'
        assert ET.fromstring(rendered).attrib.get('viewBox') == expected
        (output/'layout-preview.svg').write_text(rendered)
        initial = ' '.join(str(INITIAL_CAMERA[k]) for k in ('x', 'y', 'width', 'height'))
        (output/'initial-preview.svg').write_text(rendered.replace(f'viewBox="{expected}"', f'viewBox="{initial}"', 1))
        preview = '''<!doctype html><meta charset="utf-8"><title>DayLight native model / renderer preview</title>
<style>body{margin:0;font:14px system-ui;background:#fff}nav{height:44px;display:flex;gap:18px;align-items:center;padding:0 16px}svg{width:100vw;height:calc(100vh - 44px)}button,select{font:inherit}</style>
<nav><strong>Shared renderer preview</strong><select id="focus" aria-label="Focus section"></select>
<label><input id="details" type="checkbox">Supporting relationships</label>
<label><input id="fields" type="checkbox">Fields</label><span>Use the app for source inspection and hover tracing.</span></nav>
'''+rendered+'''<script>
const tours = '''+json.dumps([dict(name='Entry', **INITIAL_CAMERA)]+camera['tours'])+''';
const svg=document.querySelector('svg'), focus=document.querySelector('#focus');
for(const [i,tour] of tours.entries()){const o=document.createElement('option');o.value=i;o.textContent=tour.name;focus.append(o);}
function view(){const t=tours[focus.value||0];svg.setAttribute('viewBox',[t.x,t.y,t.width,t.height].join(' '));}
function details(){const show=document.querySelector('#details').checked;svg.querySelectorAll('[data-visibility="detail"]').forEach(e=>e.style.display=show?'':'none');}
function fields(){const show=document.querySelector('#fields').checked;svg.querySelectorAll('[data-diagram-detail="entity"]').forEach(e=>e.style.display=show?'':'none');svg.querySelectorAll('[data-diagram-summary="entity"]').forEach(e=>e.style.display=show?'none':'');}
focus.onchange=view;document.querySelector('#details').onchange=details;document.querySelector('#fields').onchange=fields;view();details();fields();
</script>'''
        (output/'layout-preview.html').write_text(preview)
    print(json.dumps(dict(board=str(output/'daylight-board.json'), native_entities=76, native_fields=675,
                          recorded_relationships=120, canvas=[width, height], elements=len(board['elements']))))


if __name__ == '__main__':
    main()
