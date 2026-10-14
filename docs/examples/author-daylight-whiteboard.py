#!/usr/bin/env python3
"""Author a selective, evidenced DayLight narrative using the native slide grammar.

Read-only exact source revisions; standard library only; no SVG imports, renderer
copy, publication, or inventory-coverage claim. Output must be outside the saga.
"""
import argparse
import hashlib
import json
from pathlib import Path
import xml.etree.ElementTree as ET

# These ERDs supply exact member pins and domain-level explanations. A concept
# may cite several members; its source action opens the honestly named scope.
PINS = [
    ('local-cloud-overview', 'r2'),
    ('local-search-retrieval', 'r5-viewer-safe'),
    ('local-capture-delivery', 'r5-viewer-safe'),
    ('cloud-source-lineage', 'r5'),
    ('cloud-corpus', 'r4'),
    ('cloud-project-authority', 'r5'),
    ('cloud-shared-destinations', 'r4'),
    ('local-episode-catalog', 'r5-viewer-safe'),
    ('local-episode-correlation', 'r5-viewer-safe'),
]
SELECTED_ENTITIES = {
    'search-fts': 'local-search-fts',
    'payload': 'local-pending-prompt-payloads',
    'delivery-debt': 'local-pending-project-deliveries',
    'admission': 'cloud-project-delivery-admissions',
    'hosted-search': 'cloud-prompt',
    'storage-project': 'cloud-projects',
    'policy-revision': 'cloud-project-policy-versions',
    'source-consent': 'cloud-shared-project-sources',
    'episode-catalog': 'local-episodes',
    'catalog-artifacts': 'local-episode-artifacts',
}
SELECTED_RELATIONSHIPS = {
    ('local-pending-project-deliveries', 'fk-observationid'),
    ('cloud-prompt', 'logical-admission'),
    ('cloud-projects', 'fk-2'),
    ('cloud-projects', 'fk-policy-parent'),
    ('local-episode-artifacts', 'fk-episode-id'),
}
MARKERS = {'0..many': 'zero-or-many', '0..1': 'zero-or-one', '1': 'only-one'}
INITIAL_CAMERA = dict(x=0, y=0, width=2200, height=1620)
AREAS = {
    'search-area': (60, 280, 2060, 1460),
    'delivery-area': (2260, 280, 2060, 1460),
    'authority-area': (2260, 1900, 2060, 1460),
    'episodes-area': (60, 1900, 2060, 1460),
}


def file_digest(path):
    return 'sha256:' + hashlib.sha256(path.read_bytes()).hexdigest()


def box(rect):
    return dict(zip(('x', 'y', 'width', 'height'), rect))


def overlaps(a, b):
    x, y, w, h = a
    u, v, c, d = b
    return not (x+w <= u or u+c <= x or y+h <= v or v+d <= y)


def boundary(rect, toward):
    x, y, w, h = rect
    x, y = x+w/2, y+h/2
    dx, dy = toward[0]-x, toward[1]-y
    if not dx and not dy:
        return x, y
    scale = min(w/2/abs(dx) if dx else float('inf'), h/2/abs(dy) if dy else float('inf'))
    return x+dx*scale, y+dy*scale


def segment_hits(a, b, rect):
    """An open-interior rectangle test, also used by the focused regressions."""
    x, y, w, h = rect
    x, y, w, h = x+1, y+1, w-2, h-2
    lo, hi = 0., 1.
    dx, dy = b[0]-a[0], b[1]-a[1]
    for p, q in [(-dx, a[0]-x), (dx, x+w-a[0]), (-dy, a[1]-y), (dy, y+h-a[1])]:
        if p == 0:
            if q < 0:
                return False
        elif p < 0:
            lo = max(lo, q/p)
        else:
            hi = min(hi, q/p)
    return lo < hi


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('saga', type=Path, help='read-only DayLight saga copy')
    parser.add_argument('output', type=Path, help='new directory outside the saga')
    parser.add_argument('--rendered-svg', type=Path, help='optional shared-renderer output for a standalone preview')
    args = parser.parse_args()
    root, output = args.saga.resolve(), args.output.resolve()
    if output == root or root in output.parents:
        parser.error('output must be outside the source saga')
    reads, records, entity_pins = {}, {}, {}

    def read(path):
        path = path.resolve()
        if root not in path.parents or path.suffix != '.json':
            raise ValueError('only source JSON within the saga may be read')
        reads[path] = file_digest(path)
        return json.loads(path.read_text())

    for key, revision in PINS:
        record = read(root/'___inventory'/'erds'/(key+'.erd')/'revisions'/(revision+'.json'))
        assert record['id'] == revision
        records[key] = record
        for pin in record['directory']:
            k = pin['target'].split(':')[-1]
            assert k not in entity_pins or entity_pins[k] == pin, ('conflicting pin', k)
            entity_pins[k] = pin

    def entity(key):
        if key not in records:
            pin = entity_pins[key]
            records[key] = read(root/'___inventory'/'data-entities'/(key+'.data-entity')/'revisions'/
                                (pin['revision'].split(':')[-1]+'.json'))
            assert ref(key) == pin
        return records[key]

    def ref(key):
        record = records[key]
        return dict(target=record['record'], revision=record['record']+':revision:'+record['id'])

    def style(color=None, font=34, fill=True, dash=False, width=3):
        return dict(fill=f'@diagram-{color}-tint' if color and fill else 'none',
                    stroke=f'@diagram-{color}-accent' if color else 'none',
                    ink=f'@diagram-{color}-ink' if color else '@diagram-normal-ink',
                    font_size=font, stroke_width=width, dash=dash)

    board = dict(version=1, width=4420, height=3096, viewport=INITIAL_CAMERA,
                 background='@diagram-canvas', styles={
                     'title': style(font=52), 'text': style(), 'area': style(font=40),
                     'caption': style(font=34),
                     **{c: style(c, font=36) for c in ('blue', 'purple', 'yellow', 'green', 'gray')},
                     'refusal': style('pink', dash=True),
                     'fk': style('gray', fill=False),
                     'application': style('blue', fill=False, dash=True),
                     'flow': style('yellow', fill=False, width=4),
                     'authorization': style('purple', fill=False, dash=True, width=4),
                     'failure-flow': style('pink', fill=False, dash=True),
                 }, elements=[])
    elements, bounds, paths, evidence, connections = {}, {}, {}, [], []
    bodies, labels, overlays = {}, {}, {}

    def add(key, kind, rect=None, parent=None, **attrs):
        assert key not in elements
        e = dict(id=key, kind=kind, x=0, y=0, style='text')
        e.update(attrs)
        if rect:
            e.update(box(rect))
            px, py = bounds[parent][:2] if parent else (0, 0)
            bounds[key] = (rect[0]+px, rect[1]+py, rect[2], rect[3])
        if parent:
            e['parent'] = parent
        paths[key] = (paths[parent]+'/' if parent else '')+key
        elements[key] = e
        board['elements'].append(e)
        if kind not in ('group', 'connection') and rect:
            bodies[key] = bounds[key]
        return e

    def cite(e, scope, sources, explanation, kind='concept'):
        for key in sources:
            if key not in records:
                entity(key)
        if scope not in records:
            entity(scope)
        e['source'] = ref(scope)
        e['description'] = explanation
        note = explanation if len(explanation) <= 800 else explanation.split('\n')[0]+' Full explanation is retained in the description and exact source record.'
        e['note'] = note+'\n\nEvidence scope: '+', '.join(sources)+'.'
        if ':erd:' in e['source']['target']:
            broader = ' Supporting code opens the broader pinned ERD scope, not only this depicted concept.'
            e['description'] += broader
            e['note'] += broader
        evidence.append(dict(id=e['id'], path=paths[e['id']], kind=kind, source=ref(scope),
                             scope=explanation, evidence_pins=[ref(k) for k in sources]))
        return e

    def concept(key, label, rect, parent, styling, scope, sources, explanation, shape='rect', kind='node', **attrs):
        return cite(add(key, kind, rect, parent, shape=shape, label=label, style=styling, wrap=True, **attrs),
                    scope, sources, explanation)

    def table(key, label, rect, parent, color='purple', shape='datastore'):
        source = SELECTED_ENTITIES[key]
        record = entity(source)
        e = add(key, 'node', rect, parent, shape=shape, label=label, style=color, wrap=True)
        return cite(e, source, [source], record['name']+' — selected table depiction; prose summarizes the pinned record.\n\n'+record['explanation'], 'entity')

    def text(key, label, rect, parent=None, styling='text', scope='local-cloud-overview', sources=None):
        return cite(add(key, 'text', rect, parent, label=label, wrap=True, style=styling), scope,
                    sources or [scope], 'Narrative summary of the named evidence scope; not an additional table. '+label)

    def section(key, label, rect, color, scope, parent=None):
        return cite(add(key, 'group', rect, parent, shape='section', label=label, style='area' if not parent else 'caption', color=color),
                    scope, [scope], 'Narrative area grounded in this pinned ERD; the selected members are not its full inventory.', 'area')

    def local(parent, point):
        x, y = bounds[parent][:2]
        return point[0]+x, point[1]+y

    def connect(key, owner, destination, label, parent, label_rect, bends, styling, scope,
                sources=None, relationship=None, explanation=''):
        e = add(key, 'connection', style=styling, label=label, wrap=True,
                **{'from': paths[owner], 'to': paths[destination]}, head='arrow', head_size=32,
                bends=[dict(x=x, y=y) for x, y in [local(parent, p) for p in bends]],
                label_box=box((*local(parent, label_rect[:2]), *label_rect[2:])))
        labels[key] = tuple(e['label_box'][k] for k in ('x', 'y', 'width', 'height'))
        if relationship:
            source = SELECTED_ENTITIES[owner]
            record = entity(source)
            r = next(r for r in record.get('relationships', []) if r['id'] == relationship)
            assert r['destination'] == ref(SELECTED_ENTITIES[destination])
            e.update(source={**ref(source), 'relationship': relationship},
                     tail=MARKERS[r['cardinality']['owner']], head=MARKERS[r['cardinality']['destination']],
                     description=r['label']+'\n\n'+r['explanation'], note=r['explanation'])
            connections.append(dict(id=key, recorded=True, source=e['source'], relation=r,
                                    endpoint_pins=[ref(source), r['destination']]))
        else:
            cite(e, scope, sources or [scope], explanation, 'narrative_connection')
            connections.append(dict(id=key, recorded=False, source=e['source'], explanation=explanation))
        return e

    text('title', 'DayLight / follow the data, keep the authority', (60, 35, 2080, 90), styling='title')
    text('board-thesis', 'Files retain truth; indexes derive views, delivery records debt, and access stays explicit.', (60, 125, 2080, 60))
    text('route-map', '01 Find prose   →   02 Deliver it   →   03 Govern access   →   04 Keep recording truth',
         (60, 195, 2080, 60), styling='caption')
    text('legend', 'Crow’s feet: recorded associations (gray FK / blue application).\nArrowheads: conceptual projections, transport or access checks. Selected model, not full inventory.',
         (2260, 90, 2050, 135), styling='caption')

    section('search-area', '01 / The index locates prose; it does not copy the transcript', AREAS['search-area'], 'gray', 'local-search-retrieval')
    text('search-thesis', 'Original bodies stay in their files. Metadata finds exact records; contentless FTS5 holds derived search terms.',
         (50, 90, 1960, 110), 'search-area', sources=['local-source-files', 'local-search-documents', 'local-search-fts'], scope='local-search-retrieval')
    section('device-files', 'DEVICE FILES', (50, 235, 500, 700), 'blue', 'local-search-retrieval', 'search-area')
    section('device-sqlite', 'DEVICE SQLITE / complementary projections', (800, 235, 1210, 700), 'purple', 'local-search-retrieval', 'search-area')
    concept('original-prose', 'Original prose\n\nTranscripts\nWatched files\nSource of truth', (50, 120, 400, 340), 'device-files', 'blue',
            'local-search-retrieval', ['local-source-files', 'local-search-documents'],
            'Conceptual filesystem source, not a SQL entity. source_files locates private files; search_documents records byte ranges and later hydration/verification.', shape='document')
    text('hydrate-source', 'Reopen and verify\nthe source for\nevidence.', (50, 510, 420, 145), 'device-files', scope='local-search-documents')
    concept('search-metadata', 'Source + document\nmetadata', (50, 120, 480, 350), 'device-sqlite', 'purple',
            'local-search-retrieval', ['local-source-files', 'local-search-documents'],
            'Conceptual summary of two physical tables, source_files and search_documents. This cylinder is not a third table. The source action opens their retrieval ERD.', shape='datastore')
    table('search-fts', 'Contentless FTS5\n\nCleaned search terms\nNo duplicate body', (650, 120, 500, 350), 'device-sqlite')
    text('terms-sensitive', 'Derived terms are still sensitive; this is not redaction.', (50, 535, 1120, 100), 'device-sqlite', scope='local-search-fts')
    connect('exact-coordinates', 'original-prose', 'search-metadata', 'exact\ncoordinates', 'search-area', (580, 400, 245, 95), [(650, 550)], 'application',
            'local-search-retrieval', ['local-source-files', 'local-search-documents'], explanation='Conceptual indexing projection to the two-table metadata summary, not a SQL FK.')
    connect('cleaned-terms', 'original-prose', 'search-fts', 'cleaned search terms; original prose stays in its source', 'search-area', (650, 995, 1250, 70),
            [(640, 670), (640, 975), (1990, 975), (1990, 530)], 'application', 'local-search-retrieval', ['local-search-fts', 'local-search-documents'],
            explanation='Conceptual indexing flow. The pinned FTS record contains terms only; this arrow asserts no cardinality or FK.')
    section('startup', 'STARTUP / required search capability', (50, 1090, 1960, 300), 'gray', 'local-search-retrieval', 'search-area')
    concept('search-capability', 'Requires FTS5 + contentless_delete\nNo ordinary-text fallback', (40, 90, 1120, 160), 'startup', 'gray', 'local-search-fts', ['local-search-fts'],
            'Capability requirement summarized from search_fts; this is a condition, not a persisted table.')
    concept('search-unavailable', 'Unavailable\nNo candidates', (1540, 90, 370, 160), 'startup', 'refusal', 'local-search-fts', ['local-search-fts'],
            'Unavailable search capability creates no plaintext fallback. This refusal state is not a persisted entity.')
    connect('capability-failure', 'search-capability', 'search-unavailable', 'if unavailable', 'search-area', (1240, 1145, 335, 60), [], 'failure-flow',
            'local-search-fts', explanation='Conceptual refusal branch when required FTS capability is unavailable; no SQL relationship.')

    section('delivery-area', '02 / Capture once; retire each destination’s debt', AREAS['delivery-area'], 'yellow', 'local-capture-delivery')
    text('delivery-thesis', 'One observation can owe several destinations. Cloud admission records its exact source lineage.', (50, 90, 1960, 110), 'delivery-area',
         scope='local-cloud-overview', sources=['local-pending-prompt-payloads', 'local-pending-project-deliveries', 'cloud-project-delivery-admissions'])
    table('payload', 'Contained payload\n\nOne observation\nShared across deliveries', (70, 300, 570, 320), 'delivery-area', 'blue')
    table('delivery-debt', 'Destination delivery debt\n\nGrant + generation\nRetry / status', (1260, 300, 650, 320), 'delivery-area', 'yellow')
    connect('payload-ownership', 'delivery-debt', 'payload', 'deliveries share\none payload', 'delivery-area', (710, 335, 490, 110), [], 'fk', 'local-capture-delivery', relationship='fk-observationid')
    concept('observation-callout', 'Observed byte progress is separate\nfrom destination delivery debt.', (70, 700, 760, 150), 'delivery-area', 'caption',
            'local-capture-delivery', ['local-capture-observation-offsets', 'local-pending-project-deliveries'],
            'Explanation of separate state, not an additional table or relationship.', kind='annotation', shape='bubble', color='yellow', about=paths['payload'])
    table('admission', 'Cloud admission\n\nSource grant + generation\nkept out of the prompt row', (1260, 800, 650, 280), 'delivery-area')
    connect('server-admission', 'delivery-debt', 'admission', 'server\nadmission', 'delivery-area', (1660, 660, 330, 105), [], 'flow',
            'local-cloud-overview', ['local-pending-project-deliveries', 'cloud-project-delivery-admissions'], explanation='Transport explanation: a server-admitted delivery records lineage. This is not a cross-database foreign key.')
    concept('immutable-bytes', 'Immutable source bytes', (70, 890, 650, 130), 'delivery-area', 'blue', 'cloud-corpus', ['cloud-prompt'],
            'Conceptual object storage, not a SQL table. The corpus ERD explains immutable S3 bytes; prompt.body_source is an application-validated pointer.', shape='document')
    table('hosted-search', 'Hosted search projection\n\nProject fence · source pointer', (70, 1080, 650, 250), 'delivery-area')
    connect('body-source', 'hosted-search', 'immutable-bytes', 'reads original bytes\nthrough a source pointer', 'delivery-area', (770, 975, 440, 105), [], 'application',
            'cloud-prompt', explanation='Application-validated JSON pointer to immutable source bytes, not a database FK.')
    connect('delivery-lineage', 'hosted-search', 'admission', 'recovers source lineage\nwhen an admission exists', 'delivery-area', (810, 1260, 660, 105),
            [(1000, 1205), (1585, 1205)], 'application', 'cloud-corpus', relationship='logical-admission')

    section('authority-area', '03 / Storage authority and sharing are separate', AREAS['authority-area'], 'purple', 'cloud-project-authority')
    text('authority-thesis', 'Storage ownership admits bytes. Sharing adds two checks: membership and exact-source consent.', (50, 90, 1960, 110), 'authority-area',
         scope='local-cloud-overview', sources=['cloud-projects', 'cloud-team-members', 'cloud-shared-project-sources'])
    table('storage-project', 'Storage Project\n\nPrivate capture scope\nDirect person Owner', (70, 280, 590, 300), 'authority-area')
    table('policy-revision', 'Immutable policy revision\n\nSelected by the Project', (1300, 280, 610, 300), 'authority-area', 'yellow', shape='document')
    connect('current-policy', 'storage-project', 'policy-revision', 'selects current policy', 'authority-area', (705, 345, 540, 65), [], 'fk', 'cloud-project-authority', relationship='fk-2')
    connect('policy-parent', 'storage-project', 'policy-revision', 'retains policy history', 'authority-area', (670, 720, 660, 65), [(365, 690), (1605, 690)], 'fk',
            'cloud-project-authority', relationship='fk-policy-parent')
    text('policy-directions', 'Each Project selects one current policy; past revisions remain immutable.', (70, 815, 1820, 65), 'authority-area', scope='cloud-project-policy-versions')
    concept('membership', 'Shared Project membership', (70, 990, 700, 270), 'authority-area', 'purple',
            'cloud-shared-destinations', ['cloud-teams', 'cloud-team-members'],
            'Conceptual membership check spanning the shared destination teams and its team_members. These historical teams are not people_teams; this box is not one table.')
    table('source-consent', 'Exact-source consent\n\nStorage Project + generation\nAccepted rule revision', (1210, 990, 700, 270), 'authority-area')
    concept('shared-read', 'Shared reads require\nBOTH checks', (810, 1290, 440, 140), 'authority-area', 'green', 'local-cloud-overview',
            ['cloud-team-members', 'cloud-shared-project-sources'], 'Conceptual conjunction of current membership and exact-source consent, not a table or a complete authorization algorithm.')
    connect('membership-check', 'membership', 'shared-read', 'membership', 'authority-area', (70, 1345, 460, 60), [(790, 1140), (790, 1360)], 'authorization',
            'local-cloud-overview', ['cloud-team-members'], explanation='One required input to the conceptual shared-read check; membership alone does not grant source consent.')
    connect('consent-check', 'source-consent', 'shared-read', 'source consent', 'authority-area', (1450, 1345, 530, 60), [(1560, 1320), (1310, 1320)], 'authorization',
            'local-cloud-overview', ['cloud-shared-project-sources'], explanation='The other required input to shared reads: exact-source consent does not replace membership.')
    connect('authorized-read', 'shared-read', 'hosted-search', 'membership + exact-source consent govern shared reads', 'authority-area', (890, -105, 1220, 65),
            [(1030, 1485), (2130, 1485), (2130, -125), (395, -125)], 'authorization', 'local-cloud-overview',
            ['cloud-team-members', 'cloud-shared-project-sources', 'cloud-prompt'], explanation='Narrative access rule from the cross-store overview, not a SQL relationship. It governs the hosted search projection above.')

    section('episodes-area', '04 / Recording truth stays with manifests and files', AREAS['episodes-area'], 'blue', 'local-episode-catalog')
    text('episodes-thesis', 'Manifests and hash-verified files own recording truth. The catalog is a rebuildable view.', (50, 90, 1960, 110), 'episodes-area',
         scope='local-episode-catalog', sources=['local-episodes', 'local-episode-artifacts'])
    concept('recording-files', 'Validated manifest\n+ recording files\n\nPaths · hashes · revision\nRecording authority', (70, 300, 550, 380), 'episodes-area', 'blue',
            'local-episode-catalog', ['local-episodes', 'local-episode-artifacts'],
            'Conceptual filesystem authority: validated manifest plus hash-verified artifacts. This is not a catalog table.', shape='document')
    table('episode-catalog', 'Episode catalog\n\nList / filter projection\nCan be rebuilt', (1300, 300, 610, 280), 'episodes-area')
    connect('manifest-projection', 'recording-files', 'episode-catalog', 'validated projection', 'episodes-area', (685, 365, 555, 65), [], 'application',
            'local-episode-catalog', ['local-episodes'], explanation='Conceptual projection from authoritative manifest into rebuildable episodes catalog; not a SQL FK.')
    table('catalog-artifacts', 'Catalog artifacts\n\nRelative path · bytes · SHA-256', (1300, 950, 610, 260), 'episodes-area')
    connect('artifact-owner', 'catalog-artifacts', 'episode-catalog', 'belongs to\none episode', 'episodes-area', (1670, 710, 300, 110), [], 'fk',
            'local-episode-catalog', relationship='fk-episode-id')
    concept('deletion-callout', 'A catalog cascade removes metadata.\nDeleting files is a separate\napplication lifecycle operation.', (70, 920, 850, 190), 'episodes-area', 'caption',
            'local-episode-artifacts', ['local-episode-artifacts'], 'Annotation of the artifact FK: physical metadata cascade does not claim filesystem deletion.',
            kind='annotation', shape='bubble', color='yellow', about=paths['catalog-artifacts'])
    cite(add('derived-evidence', 'sticky', (70, 1260, 1840, 140), 'episodes-area', label='Search may add versioned episode evidence: basis, confidence and timing.\nA derived correlation does not replace the manifest.', style='caption', color='gray', wrap=True,
             about=paths['episode-catalog']), 'local-episode-evidence-links', ['local-episode-evidence-links'],
         'Conceptual annotation of the separate search-side evidence_links projection. It is not another episode-catalog table.')

    # Meaningful member prose has its own exact source. Aggregate body actions
    # retain the broader ERD scope; no one member stands in for the whole.
    for key, label, rect, parent, scope, aggregate in [
        ('metadata-file-evidence', 'File identity + cursor', (85, 285, 420, 60), 'device-sqlite', 'local-source-files', 'search-metadata'),
        ('metadata-record-evidence', 'Record key + byte range', (85, 380, 420, 60), 'device-sqlite', 'local-search-documents', 'search-metadata'),
        ('destination-evidence', 'Shared destination', (105, 1090, 630, 60), 'authority-area', 'cloud-teams', 'membership'),
        ('person-evidence', 'A person + current evidence', (105, 1170, 630, 60), 'authority-area', 'cloud-team-members', 'membership'),
    ]:
        text(key, label, rect, parent, scope=scope)
        overlays[key] = aggregate
        evidence[-1]['member_of'] = paths[aggregate]
    connect('transcript-capture', 'original-prose', 'payload', 'captured transcript observations only', 'search-area', (2270, 225, 1100, 65),
            [(-40, 525), (-40, -12), (2140, -12), (2140, 460)], 'flow',
            'local-pending-prompt-payloads', ['local-pending-prompt-payloads', 'local-source-files'],
            explanation='Scoped conceptual capture of transcript observations, independent of local indexing. This does not assert that every watched file or search result is delivered, and search membership grants no delivery authority.')
    connect('episode-context', 'search-metadata', 'episode-catalog', 'search may correlate a record with an episode', 'search-area', (70, 1525, 1720, 65),
            [(1090, 330), (-20, 330), (-20, 1500), (2120, 1500), (2120, 2060)], 'application',
            'local-episode-evidence-links', ['local-episode-evidence-links', 'local-search-documents', 'local-episodes'],
            explanation='Conceptual episode context for a search record, with versioned basis, confidence and timing in evidence_links. The catalog represents manifest provenance here; this arrow is not a direct FK or a claim that indexing creates recording authority.')

    # Reserve room for app chrome on a 1440 x 1000 desktop: the authored
    # coordinates compress vertically, while all prose remains 34 units or more.
    # This is native geometry, not a scaled frame with shrinking text.
    for e in board['elements']:
        e['y'] *= .9
        if 'height' in e:
            e['height'] *= .9
        for p in e.get('bends', []):
            p['y'] *= .9
        if 'label_box' in e:
            e['label_box']['y'] *= .9
            e['label_box']['height'] *= .9
    for collection in (bounds, bodies, labels):
        for key, (x, y, w, h) in collection.items():
            collection[key] = (x, y*.9, w, h*.9)

    # Explicit geometry stays simple: authored straight lines and a few doglegs.
    # No automatic routing or world-size expansion to accommodate an inventory.
    for key, rect in bodies.items():
        x, y, w, h = rect
        assert min(x, y) >= 0 and x+w <= board['width'] and y+h <= board['height'], ('outside', key)
    for i, (key, rect) in enumerate(bodies.items()):
        for other, other_rect in list(bodies.items())[i+1:]:
            if overlays.get(key) != other and overlays.get(other) != key:
                assert not overlaps(rect, other_rect), ('body overlap', key, other)
    for key, rect in labels.items():
        for other, other_rect in {**bodies, **labels}.items():
            if key != other:
                assert not overlaps(rect, other_rect), ('label overlap', key, other)
    for e in board['elements']:
        if e['kind'] != 'connection':
            continue
        a, b = (e[k].split('/')[-1] for k in ('from', 'to'))
        bends = [(p['x'], p['y']) for p in e['bends']]
        ca = (bounds[a][0]+bounds[a][2]/2, bounds[a][1]+bounds[a][3]/2)
        cb = (bounds[b][0]+bounds[b][2]/2, bounds[b][1]+bounds[b][3]/2)
        points = [boundary(bounds[a], bends[0] if bends else cb)]+bends+[boundary(bounds[b], bends[-1] if bends else ca)]
        for key, rect in {**bodies, **labels}.items():
            if key not in (a, b):
                assert not any(segment_hits(u, v, rect) for u, v in zip(points, points[1:])), ('route collision', e['id'], key)
        for endpoint, point in [(a, points[0]), (b, points[-1])]:
            # Includes the arrow/crowfoot plus room around its clipped tip.
            marker = (point[0]-36, point[1]-36, 72, 72)
            for key, rect in {**bodies, **labels}.items():
                if key != endpoint and overlays.get(key) != endpoint:
                    assert not overlaps(marker, rect), ('marker collision', e['id'], key)
    groups = [e for e in board['elements'] if e['kind'] == 'group']
    for i, group in enumerate(groups):
        for other in groups[i+1:]:
            if group.get('parent') == other.get('parent'):
                assert not overlaps(bounds[group['id']], bounds[other['id']]), ('group overlap', group['id'], other['id'])
    for child, host in overlays.items():
        x, y, w, h = bounds[child]
        hx, hy, hw, hh = bounds[host]
        assert hx <= x and hy <= y and x+w <= hx+hw and y+h <= hy+hh, ('member overlay escapes', child)
    for e in board['elements']:
        if e.get('parent'):
            x, y, w, h = bounds[e['id']]
            px, py, pw, ph = bounds[e['parent']]
            assert px <= x and py <= y and x+w <= px+pw and y+h <= py+ph, ('parent containment', e['id'])
    assert {(c['source']['target'].split(':')[-1], c['source']['relationship']) for c in connections if c['recorded']} == SELECTED_RELATIONSHIPS
    assert len(board['elements']) < 90
    for path, digest in reads.items():
        assert file_digest(path) == digest, ('source changed', path)

    used = ({p['target'] for e in evidence for p in e['evidence_pins']} |
            {e['source']['target'] for e in evidence} | {c['source']['target'] for c in connections})
    source_records = [dict(source=ref(k), record=r) for k, r in records.items() if r['record'] in used]
    audit = dict(mode='selective narrative; intentionally not inventory coverage',
                 source_limit='Pinned committed-code documentation, not live database inspection. No slide Item or story bindings are claimed.', selected_entities=SELECTED_ENTITIES,
                 selected_recorded_relationships=len(SELECTED_RELATIONSHIPS), narrative_connections=sum(not c['recorded'] for c in connections),
                 elements=len(board['elements']), areas=[dict(id=k, path=paths[k], **box(bounds[k])) for k in AREAS],
                 evidence=evidence, source_modified=False, source_svg_required=False, published=False,
                 detail_policy='Native prose is readable without Details. Selected table and relationship source actions retain exact full records; omitted inventory is not implied.',
                 source_reads=[dict(path=str(p.relative_to(root)), digest=d) for p, d in reads.items()])
    camera = dict(initial=INITIAL_CAMERA, canvas=dict(width=board['width'], height=board['height']),
                  tours=[dict(name=elements[k]['label'], anchor=paths[k], **box(bounds[k])) for k in AREAS])
    output.mkdir(parents=True, exist_ok=True)
    for name, data in [('daylight-board.json', board), ('source-audit.json', audit), ('connection-evidence.json', connections),
                       ('entity-evidence.json', source_records), ('source-pins.json', [ref(k) for k in records if records[k]['record'] in used]), ('camera-proposal.json', camera)]:
        (output/name).write_text(json.dumps(data, indent=2)+'\n')
    if args.rendered_svg:
        rendered = args.rendered_svg.read_text()
        assert ET.fromstring(rendered).attrib.get('viewBox') == '0 0 4420 3096'
        (output/'layout-preview.html').write_text('''<!doctype html><meta charset="utf-8"><title>DayLight narrative / shared renderer</title>
<style>body{margin:0;background:var(--diagram-canvas,#fff);color:var(--diagram-normal-ink);font:14px system-ui}nav{height:40px;padding:0 16px;display:flex;align-items:center;gap:12px}svg{width:100vw;height:calc(100vh - 40px)}</style>
<nav>Shared renderer preview <select id="focus"></select></nav>'''+rendered+'''<script>
const tours='''+json.dumps([dict(name='Opening', **INITIAL_CAMERA)]+camera['tours'])+''';
const svg=document.querySelector('svg'),select=document.querySelector('#focus');
for(const [i,t] of tours.entries())select.add(new Option(t.name,i));
function view(){const t=tours[select.value||0];svg.setAttribute('viewBox',[t.x,t.y,t.width,t.height].join(' '));}select.onchange=view;view();
</script>''')
    print(json.dumps(dict(board=str(output/'daylight-board.json'), selected_entities=len(SELECTED_ENTITIES), recorded_relationships=len(SELECTED_RELATIONSHIPS), elements=len(board['elements']))))


if __name__ == '__main__':
    main()
