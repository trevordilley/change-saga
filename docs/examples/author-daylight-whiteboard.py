#!/usr/bin/env python3
"""Author a pinned DayLight composition from a read-only local saga copy.

Python standard library only. Never publishes or modifies the input saga.
Exact revision IDs below are intentional: this example must not follow heads.
"""
import argparse
import hashlib
import json
from pathlib import Path
import xml.etree.ElementTree as ET

# Exact migration/provenance pins. These drawings live in the appendix; the
# main document is authored from their native entity/relationship records.
PINS = [
    ('local-cloud-overview', 'r2', 'The system across its stores'),
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
    ('cloud-overview', 'r3', 'Whole cloud model'),
]
WIDTH, HEIGHT = 14400, 24300
INITIAL_CAMERA = dict(x=0, y=0, width=6200, height=4900)
CAPTION_CLEARANCE = 160


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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('saga', type=Path, help='read-only DayLight saga copy')
    parser.add_argument('output', type=Path, help='new output directory outside the saga')
    parser.add_argument('--rendered-svg', type=Path,
                        help='shared composition-renderer SVG for diagnostic previews')
    args = parser.parse_args()
    root, output = args.saga.resolve(), args.output.resolve()
    if output == root or root in output.parents:
        parser.error('output must be outside the source saga')
    output.mkdir(parents=True, exist_ok=True)
    records, images, manifest, source_reads = {}, {}, [], {}
    for key, revision, *_ in PINS:
        directory = root / '___inventory' / 'erds' / (key + '.erd')
        matches = [(p, json.loads(p.read_text())) for p in sorted((directory / 'revisions').glob('*.json'))]
        matches = [(p, r) for p, r in matches if r['id'] == revision]
        if len(matches) != 1:
            raise ValueError(f'{key}: expected exact revision {revision}')
        revision_path, record = matches[0]
        records[key] = record
        revision_digest = 'sha256:' + hashlib.sha256(revision_path.read_bytes()).hexdigest()
        source_reads[revision_path] = revision_digest
        asset = (directory / record['visual']['path']).resolve()
        if root not in asset.parents:
            raise ValueError(f'{key}: asset escapes source saga')
        svg = asset.read_bytes()
        digest = 'sha256:' + hashlib.sha256(svg).hexdigest()
        if digest != record['visual']['digest']:
            raise ValueError(f'{key}: source SVG digest mismatch')
        source_reads[asset] = digest
        viewbox = [float(n) for n in ET.fromstring(svg).attrib['viewBox'].split()]
        images[key] = (svg, viewbox[2], viewbox[3])
        manifest.append({'id': key, 'target': record['record'],
                         'revision': record['record'] + ':revision:' + revision,
                         'visual_digest': digest, 'revision_digest': revision_digest, 'intrinsic_width': viewbox[2],
                         'intrinsic_height': viewbox[3], 'explanation': record['explanation']})


    def ref(key):
        r = records[key]
        return {'target': r['record'], 'revision': r['record'] + ':revision:' + r['id']}

    # Read exact entity revisions named by the pinned directories, never heads.
    entities, appearances = {}, {}
    for erd, record in records.items():
        for pin in record['directory']:
            target = pin['target'].split(':')[-1]
            revision = pin['revision'].split(':')[-1]
            path = root/'___inventory'/'data-entities'/(target+'.data-entity')/'revisions'/(revision+'.json')
            entity = json.loads(path.read_text())
            assert entity['record'] == pin['target'] and entity['id'] == revision
            source_reads[path] = file_digest(path)
            # This pinned corpus uses one entity revision per entity everywhere.
            if target in entities:
                assert entities[target] == entity, f'conflicting entity pins: {target}'
            entities[target] = entity
            appearances.setdefault(target, []).append(erd)

    def entity_ref(key):
        r = entities[key]
        return dict(target=r['record'], revision=r['record']+':revision:'+r['id'])

    def style(fill='none', stroke='none', ink='@diagram-normal-ink', font=64, width=3):
        return dict(fill=fill, stroke=stroke, ink=ink, font_size=font, stroke_width=width)

    board = dict(version=1, width=WIDTH, height=HEIGHT, viewport=INITIAL_CAMERA,
                 background='@diagram-canvas', styles={
        'entry': style(font=120), 'hero': style(font=180), 'heading': style(font=144), 'body': style(font=90),
        'coverage': style(font=100), 'small': style(font=74), 'area': style(font=100),
        'entity': style('@diagram-normal-fill', '@diagram-normal-stroke', font=120, width=4),
        'frame': style('@diagram-normal-fill', '@diagram-normal-stroke', font=64),
        'fk': style(stroke='@diagram-normal-stroke', font=96, width=5),
        'projection': dict(**style(stroke='@diagram-blue-accent', font=96, width=5), dash=True),
        'transport': style(stroke='@diagram-yellow-accent', font=96, width=8),
        'authorization': dict(**style(stroke='@diagram-purple-accent', font=96, width=6), dash=True),
        'appearance': dict(**style(stroke='@diagram-secondary-stroke', font=96, width=3), dash=True),
    }, elements=[])
    by_id, paths, bodies, labels, evidence = {}, {}, {}, {}, []

    def add(e):
        assert e['id'] not in by_id, e['id']
        board['elements'].append(e)
        by_id[e['id']] = e
        paths[e['id']] = (paths[e['parent']]+'/' if e.get('parent') else '')+e['id']
        return e

    def box(x, y, w, h):
        return dict(x=x, y=y, width=w, height=h)

    def bounds(e):
        x, y = e['x'], e['y']
        parent = e.get('parent')
        while parent:
            p = by_id[parent]
            x, y = x+p['x'], y+p['y']
            parent = p.get('parent')
        return (x, y, e.get('width', 0), e.get('height', 0))

    def text(key, label, x, y, w, h, source, styling='body'):
        e = add(dict(id=key, kind='text', label=label, **box(x,y,w,h),
                     style=styling, wrap=True, source=ref(source)))
        bodies[key] = bounds(e)

    def section(key, label, color, rect, source, parent=None):
        e = dict(id=key, kind='group', shape='section', label=label, color=color,
                 **box(*rect), style='area', source=ref(source))
        if parent:
            e['parent'] = parent
        add(e)

    section('context-area', 'One system / three independent kinds of state', 'gray',
            (100,100,14200,1350), 'local-cloud-overview')
    section('delivery-area', '01 / Capture, exact admission, derived corpus', 'yellow',
            (100,1600,14200,3800), 'local-capture-delivery')
    section('search-area', '02 / Local search and episode evidence', 'blue',
            (100,5900,6200,7800), 'local-search-retrieval')
    section('authority-area', '03 / Storage ownership, membership and consent', 'purple',
            (7000,5900,7300,7800), 'cloud-project-authority')
    section('provenance-area', 'Source evidence / original pinned drawings', 'gray',
            (100,14300,14200,9900), 'cloud-overview')
    # True nested groups; the stable root section IDs remain viewer focus targets.
    section('capture', 'Device / observation and destination debt', 'yellow',
            (180,620,5700,2950), 'local-capture-delivery', 'delivery-area')
    section('hosted', 'Hosted / source identity and delivery admission', 'yellow',
            (6380,620,7520,2950), 'cloud-source-lineage', 'delivery-area')
    section('local-scope', 'Durable choices become query scope', 'blue',
            (180,650,5820,3200), 'local-project-boundary', 'search-area')
    section('episodes', 'Validated manifests become evidence projections', 'blue',
            (180,4150,5820,3350), 'local-episode-correlation', 'search-area')
    section('storage', 'Private storage authority', 'purple',
            (180,650,6820,1700), 'cloud-project-authority', 'authority-area')
    section('sharing', 'Shared access / separate checks', 'purple',
            (180,2650,6820,2850), 'cloud-contribution', 'authority-area')
    section('directory', 'Principal and Organization context', 'purple',
            (180,5800,6820,1700), 'cloud-identity', 'authority-area')

    text('title', 'DayLight / one connected data model', 350,290,5450,280,
         'local-cloud-overview','hero')
    text('intro', 'Local work produces destination-specific debt. Admission preserves the exact source generation. Search is derived; membership and consent govern access.',
         350,650,5450,420,'local-cloud-overview','entry')
    text('selection-note', '23 core tables lead the story. All 76 tables and all 20 source drawings remain below in Source evidence.',
         350,1100,5450,300,'cloud-overview','coverage')
    text('context-cloud', 'Continue right / hosted admission', 6600,320,3350,330,'cloud-source-lineage','heading')
    text('context-access', 'Continue below / search and access', 10600,320,3250,330,'local-cloud-overview','heading')
    text('legend', 'Gray: physical FK · Blue dashed: application · Amber: transport · Purple: access checks',
         6600,830,7100,300,'local-cloud-overview','small')
    text('delivery-story', 'Observation progress is separate from destination debt. Follow the same source grant and generation into hosted admission →',
         350,1810,5500,300,'local-cloud-overview','entry')
    text('search-story', 'Local retrieval remains independent of cloud delivery. A file cursor is not an acknowledgement.',
         350,6090,5650,240,'local-index-progress')
    text('authority-story', 'Storage Projects, shared Projects and people Teams have different authority. A matching label grants no access.',
         7250,6090,6700,240,'cloud-shared-destinations')

    # These source-backed summaries fill the native overview body. The viewer
    # shows them only with Details off, so they cannot obscure full field rows.
    summaries = {
        'local-capture-observation-offsets': 'Observed byte progress, independent of cloud receipts.',
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
        'cloud-organizations': 'Organization authority and lifecycle for people Teams.',
    }
    nodes = {}
    def node(key, label, parent, x, y, width=1900):
        r = entities[key]
        fields=[]
        for f in r['fields']:
            keys=f.get('keys', [])
            field=dict(name=f['name'],type=f['type'])
            k=','.join(k for name,k in [('primary','pk'),('foreign','fk')] if name in keys)
            if k: field['key']=k
            fields.append(field)
        # The renderer owns fixed 28-unit field rows. Keep the full native fields
        # for Details; the overview shows the large title in these same bounds.
        height=max(560, 120*1.25+16+28*len(fields)+20)
        px,py,_,_=bounds(by_id[parent])
        e=add(dict(id=key,kind='node',shape='entity',label=label,
                   **box(x-px,y-py,width,height),parent=parent,style='entity',
                   fields=fields,summary=summaries[key],source=entity_ref(key),description=r['explanation'],
                   note='**'+r['name']+'**\n\n'+r['explanation'].split('\n\n')[0][:850]))
        nodes[key]=paths[key]
        bodies[key]=bounds(e)

    # One connected native model, not one primary frame per former ERD.
    for placement in [
        ('local-capture-observation-offsets','Observation progress','capture',500,2450),
        ('local-project-source-grants','Source grant / generation','capture',3700,2450),
        ('local-pending-prompt-payloads','Captured payload','capture',500,4100),
        ('local-pending-project-deliveries','Destination delivery debt','capture',3700,4100),
        ('cloud-project-sources','Admitted source / generation','hosted',6800,2450),
        ('cloud-project-delivery-admissions','Admitted delivery','hosted',6800,4100),
        ('cloud-prompt','Hosted search projection','hosted',11200,4100),
        ('local-local-projects','Durable local Project','local-scope',500,6800),
        ('local-local-project-file-memberships','Projected query membership','local-scope',3900,6800),
        ('local-source-files','Indexed source file','local-scope',500,8550),
        ('local-search-documents','Retrievable document','local-scope',3900,8550),
        ('local-episodes','Manifest-backed episode','episodes',500,10400),
        ('local-episode-correlation-targets','Episode target projection','episodes',3900,10400),
        ('local-episode-artifacts','Catalog artifact','episodes',500,12300),
        ('local-episode-evidence-links','Versioned evidence link','episodes',3900,12300),
        ('cloud-projects','Private storage Project','storage',7500,6800),
        ('cloud-project-policy-versions','Immutable policy revision','storage',11500,6800),
        ('cloud-teams','Shared Project (teams)','sharing',7500,8850),
        ('cloud-shared-project-sources','Exact-source consent','sharing',11500,8850),
        ('cloud-team-members','Shared Project membership','sharing',7500,10500),
        ('cloud-team-source-rule-versions','Accepted sharing rules','sharing',11500,10500),
        ('cloud-users','Principal','directory',7500,12000),
        ('cloud-organizations','Organization','directory',11500,12000),
    ]: node(*placement)
    text('source-bytes', 'Immutable source bytes / object storage',11200,2520,2300,240,'cloud-corpus','body')
    text('source-bytes-story', 'S3 retains source bytes; PostgreSQL holds scope, identity and derived search state.',11200,2880,2300,350,'cloud-corpus')
    text('details-note', 'Select a table or line for exact source evidence. Details restores full native fields. Continue below for every supporting table and relationship.',
         350,13850,13600,240,'cloud-overview','small')

    def connection(key, start, end, label, kind, rect, bends, source, support, relation=None):
        # start/end are exact native instance paths. No SVG geometry is inferred.
        source=dict(source)
        if relation: source['relationship']=relation['id']
        e=add(dict(id=key,kind='connection',x=0,y=0,
                   **{'from':nodes.get(start,paths.get(start,start)), 'to':nodes.get(end,paths.get(end,end))},
                   label=label,label_box=box(*rect),style=kind,head='arrow',wrap=True,
                   bends=[dict(x=x,y=y) for x,y in bends],source=source,
                   description=support, note=support))
        labels[key]=rect
        evidence.append(dict(id=key,kind={'fk':'physical_fk','projection':'application_projection',
                                        'transport':'transport_explanation','authorization':'authorization_explanation',
                                        'appearance':'same_entity_appearance'}[kind],
                             source=source, **{'from':e['from'],'to':e['to']},
                             supporting_text=support,
                             code=relation.get('code',[]) if relation else [],
                             endpoint_pins=[by_id[start]['source'],by_id[end]['source']],
                             endpoint_geometry='authored native bounds' if start in nodes and end in nodes else 'whole region'))

    def rel(owner, rid, label, rect, bends=()):
        r=next(r for r in entities[owner]['relationships'] if r['id']==rid)
        destination=r['destination']['target'].split(':')[-1]
        assert entity_ref(destination)==r['destination'], f'relationship pin differs: {owner}/{rid}'
        kind='fk' if r['explanation'].lower().startswith('physical') else 'projection'
        connection(owner+'--'+rid,owner,destination,label,kind,rect,bends,entity_ref(owner),r['explanation'],r)

    rel('local-pending-prompt-payloads','observed-file','application transaction', (1520,3450,900,270))
    rel('local-pending-project-deliveries','fk-observationid','FK / observationId', (2480,4100,1100,200))
    rel('local-pending-project-deliveries','admitting-grant','copied authority tuple', (4720,3450,900,270))
    connection('same-source-generation','local-project-source-grants','cloud-project-sources',
               'same grant / generation','projection',(5670,2420,1040,250),(),ref('local-cloud-overview'),records['local-cloud-overview']['explanation'])
    connection('delivery-admission','local-pending-project-deliveries','cloud-project-delivery-admissions',
               'server admission','transport',(5670,4050,1040,230),(),ref('local-cloud-overview'),records['local-cloud-overview']['explanation'])
    connection('bytes-project-into-search','source-bytes-story','cloud-prompt',
               'derived from source bytes','transport',(12500,3470,1350,250),(),ref('cloud-corpus'),records['cloud-corpus']['explanation'])
    rel('cloud-project-delivery-admissions','fk-2','FK / source_grant_id',(6700,3450,950,180))
    rel('cloud-prompt','logical-admission','application join / delivery_id',(9500,4650,1650,250))
    rel('cloud-project-sources','fk-1','FK / storage Project',(7250,5470,1580,200),[(8900,2770),(8900,5750),(7070,5750),(7070,7090)])
    rel('cloud-projects','fk-2','FK / current policy',(9570,6600,1750,200))
    rel('cloud-shared-project-sources','fk-7','FK / exact source lineage',(13150,7700,650,400),[(13950,9200),(13950,5300),(9150,5300),(9150,2750)])
    rel('cloud-shared-project-sources','fk-2','FK / storage Project',(9500,7800,1600,200),[(10500,8580),(10500,8250),(8450,8250)])
    rel('cloud-shared-project-sources','fk-1','FK / shared destination',(10100,9360,1250,220))
    rel('cloud-shared-project-sources','fk-6','FK / accepted rule',(12520,9760,1500,180))
    rel('cloud-team-members','fk-1','FK / membership',(8550,9760,1200,180))
    rel('cloud-team-members','fk-2','FK / principal',(8550,11200,1200,180))
    rel('cloud-teams','fk-1','FK / Organization',(10080,11450,1300,220),[(9850,9160),(9850,12280)])
    connection('read-authorization','cloud-shared-project-sources','cloud-prompt',
               'read access: membership + exact-source consent','authorization',(12300,5460,1500,270),[(10900,9160),(10900,8600),(11350,8600),(11350,6470),(14070,6470),(14070,5780),(12150,5780)],ref('local-cloud-overview'),records['local-cloud-overview']['explanation'])
    rel('local-local-project-file-memberships','project-projection','application / durable choices',(2470,6610,1300,230))
    rel('local-local-project-file-memberships','member-file','application / file identity',(3220,7760,600,430),[(3150,7160),(3150,8390),(2500,8390),(2500,8850)])
    rel('local-search-documents','fk-file-id','FK / file_id',(2590,8470,1210,200))
    rel('local-episode-correlation-targets','manifest-projection','projection / manifest revision',(2480,10300,1350,250))
    rel('local-episode-artifacts','fk-episode-id','FK / episode_id',(1530,11400,1700,180))
    rel('local-episode-evidence-links','episode-edge','application / episode identity',(4960,11420,1240,250))
    rel('local-episode-evidence-links','fk-document-rowid','FK / document_rowid',(6440,9820,900,430),[(6350,12670),(6350,8890)])
    connection('storage-binding','local-local-projects','cloud-projects','optional hosted storage binding','projection',
               (2400,5500,3000,240),[(320,7120),(320,5750),(7040,5750),(7040,7420),(8450,7420)],ref('local-cloud-overview'),records['local-cloud-overview']['explanation'])

    # All original pins remain inspectable, including every ancillary entity,
    # outgoing relationship, code reference and rich local annotation. Their
    # ordering follows the main narrative. Whole-model mirrors are appendix-only.
    text('provenance-story','Supporting detail / exact source drawings',350,14520,13400,190,'cloud-overview','heading')
    text('provenance-note','The model above unifies the core entities. These pinned source drawings preserve all 76 tables, all documented relationships and the original annotations; they are evidence views of the same system.',
         350,14800,13400,250,'cloud-overview')
    frames={}
    for i,(key,rev,label) in enumerate(PINS):
        col,row=i%4,i//4
        x,y,w=350+col*3500,15400+row*1730,2850
        _,sw,sh=images[key]
        # Fit within the appendix row without distorting intrinsic aspect ratio.
        scale=min(w/sw,1380/sh)
        w,h=sw*scale,sh*scale
        e=add(dict(id=key,kind='frame',label=label,**box(x,y,w,h),style='frame',
                   detail_scale=.6,source=ref(key),description=records[key]['explanation']))
        frames[key]=bounds(e)
        bodies[key]=(x,y-CAPTION_CLEARANCE,w,h+CAPTION_CLEARANCE)
    principal_binding=next(b for b in records['cloud-identity']['bindings'] if b.get('entity')==entity_ref('cloud-users'))
    connection('same-principal-evidence','cloud-users','cloud-identity',
               'same principal / original context','appearance',(7400,13300,2600,180),
               [(7100,12280),(7100,13600),(14200,13600),(14200,19140)],entity_ref('cloud-users'),
               'The native Principal and the cloud-identity binding both pin '+entity_ref('cloud-users')['revision']+'. This is the same entity appearance, not a foreign key.')
    evidence[-1]['supporting_binding']=principal_binding
    evidence[-1]['supporting_erd']=ref('cloud-identity')

    # Verify caption/body/label rectangles and authored routes. Connection labels
    # are separate from paths, so no label may cover any entity or narrative body.
    for key,rect in {**bodies,**labels}.items():
        x,y,w,h=rect
        assert x>=0 and y>=0 and x+w<=WIDTH and y+h<=HEIGHT, ('outside canvas',key)
    for i,(key,rect) in enumerate(bodies.items()):
        for other,other_rect in list(bodies.items())[i+1:]:
            assert not overlaps(rect,other_rect), f'body overlap: {key}, {other}'
    for key,rect in labels.items():
        for other,other_rect in bodies.items():
            assert not overlaps(rect,other_rect), f'label/body overlap: {key}, {other}'
        for other,other_rect in labels.items():
            if key!=other: assert not overlaps(rect,other_rect), f'label overlap: {key}, {other}'
    for e in board['elements']:
        if e['kind'] != 'connection':
            continue
        start, end = (e[k].split('/')[-1] for k in ('from','to'))
        a, b = bounds(by_id[start]), bounds(by_id[end])
        bends = [(p['x'],p['y']) for p in e['bends']]
        points = [boundary(a,bends[0] if bends else center(b))]+bends+[boundary(b,bends[-1] if bends else center(a))]
        for key, rect in {**bodies,**labels}.items():
            if key in (start,end):
                continue
            assert not any(segment_hits(a,b,rect) for a,b in zip(points,points[1:])), f'route collision: {e["id"]}, {key}'
    for e in board['elements']:
        if e['kind'] == 'node':
            parent = bounds(by_id[e['parent']])
            child = bounds(e)
            assert parent[0]+100 <= child[0] and child[0]+child[2] <= parent[0]+parent[2]-100, e['id']
            assert parent[1]+150 <= child[1] and child[1]+child[3] <= parent[1]+parent[3]-100, e['id']
    for path,expected in source_reads.items():
        assert file_digest(path)==expected, f'source changed: {path}'
    assert len(frames)==20
    covered={p['target'] for r in records.values() for p in r['directory']}
    assert covered=={r['record'] for r in entities.values()}
    audit=dict(unique_pins=len(manifest),frame_instances=len(frames),primary_native_entities=len(nodes),
               covered_entities=len(covered),native_fields=sum(len(entities[k]['fields']) for k in nodes),
               verified_source_files=len(source_reads),all_visual_digests_verified=True,
               source_modified=False,published=False,connections=len(evidence),
               source_pins='source-pins.json',connection_evidence='connection-evidence.json',
               geometry_checks=['body/caption separation','label/body separation','label/label separation',
                                'route/body clearance','route/label clearance','native section gutters'],
               source_limit='Committed-code documentation, not live database inspection. Original SVG pins have no structured endpoint bounds.',
               detail_policy='Native core entities carry every field; all other entities and all relationships remain in exact source appendix drawings and source inspection.',
               sections=[dict(id=k,label=by_id[k]['label']) for k in ['context-area','delivery-area','search-area','authority-area','provenance-area']],
               entity_coverage=[dict(**entity_ref(k),primary_instance=nodes.get(k),source_appearances=v) for k,v in appearances.items()])
    camera=dict(initial=INITIAL_CAMERA,canvas=dict(width=WIDTH,height=HEIGHT),
                tours=[dict(name=k,anchor=k,**box(*bounds(by_id[k]))) for k in ['delivery-area','search-area','authority-area','provenance-area']])
    for name,data in [('daylight-board.json',board),('source-pins.json',manifest),
                      ('source-audit.json',audit),('connection-evidence.json',evidence),('camera-proposal.json',camera)]:
        (output/name).write_text(json.dumps(data,indent=2)+'\n')
    if args.rendered_svg:
        rendered=args.rendered_svg.read_text()
        expected=f'0 0 {WIDTH} {HEIGHT}'
        assert ET.fromstring(rendered).attrib.get('viewBox')==expected
        (output/'layout-preview.svg').write_text(rendered)
        initial=' '.join(str(INITIAL_CAMERA[k]) for k in ('x','y','width','height'))
        (output/'initial-preview.svg').write_text(rendered.replace(f'viewBox="{expected}"',f'viewBox="{initial}"',1))
        (output/'layout-preview.html').write_text('<!doctype html><meta charset="utf-8"><title>DayLight system model</title><style>body{margin:0}img{width:100vw;height:100vh;object-fit:contain}</style><img src="initial-preview.svg" alt="Connected DayLight system model">')
    print(json.dumps(dict(board=str(output/'daylight-board.json'),audit=audit),indent=2))


if __name__ == '__main__':
    main()
