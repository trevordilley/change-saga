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

# Exact source pins. Placement is a narrative, not an auto-layout or index grid.
# id, revision, title, x, y, width (all in board units, no fit-all scaling).
AREAS = [
    ('local-cloud-overview', 'r2', 'The system across its stores', 160, 920, 1550),
    ('local-capture-delivery', 'r5-viewer-safe', 'Capture & delivery', 4050, 1050, 1500),
    ('cloud-source-lineage', 'r5', 'Source admission', 6050, 1050, 1500),
    ('cloud-corpus', 'r4', 'Cloud corpus', 8050, 1050, 1500),
    ('local-project-boundary', 'r5-viewer-safe', 'Project scope', 160, 3800, 1250),
    ('local-index-progress', 'r5-viewer-safe', 'Index progress', 1710, 3800, 1250),
    ('local-search-retrieval', 'r5-viewer-safe', 'Local search', 3260, 3800, 1400),
    ('local-episode-catalog', 'r5-viewer-safe', 'Episode catalog', 160, 5500, 1400),
    ('local-episode-correlation', 'r5-viewer-safe', 'Episode links', 1860, 5500, 1450),
    ('local-whole-model', 'r5-viewer-safe', 'Whole local model', 3610, 5700, 1650),
    ('cloud-project-authority', 'r5', 'Project authority', 5700, 4300, 1200),
    ('cloud-identity', 'r4', 'Identity & tenancy', 7300, 4300, 1100),
    ('cloud-contribution', 'r4', 'Sharing rules', 8800, 4300, 1000),
    ('cloud-shared-destinations', 'r4', 'Projects & Teams', 5700, 6300, 1200),
    ('cloud-oauth', 'r4', 'MCP grants', 7300, 6300, 1100),
    ('cloud-invitations', 'r4', 'Invitations', 8800, 6000, 1000),
    ('cloud-enterprise', 'r4', 'Enterprise', 160, 8400, 1250),
    ('cloud-scim', 'r4', 'SCIM directory', 1710, 8400, 1250),
    ('cloud-messaging', 'r5', 'Durable messages', 3260, 8400, 1400),
    ('cloud-overview', 'r3', 'Whole cloud model', 4960, 8300, 1500),
]
WIDTH, HEIGHT = 10000, 10000
CAPTION_CLEARANCE = 160
INITIAL_CAMERA = dict(x=0, y=0, width=3600, height=2400)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('saga', type=Path, help='read-only DayLight saga copy')
    parser.add_argument('output', type=Path, help='new output directory outside the saga')
    parser.add_argument('--rendered-svg', type=Path,
                        help='shared composition-renderer SVG for optional diagnostic previews')
    args = parser.parse_args()
    root, output = args.saga.resolve(), args.output.resolve()
    if output == root or root in output.parents:
        parser.error('output must be outside the source saga')
    output.mkdir(parents=True, exist_ok=True)
    records, images, manifest, source_reads = {}, {}, [], {}
    for key, revision, *_ in AREAS:
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

    def style(fill='none', stroke='none', ink='@diagram-normal-ink', font=64, width=3):
        return dict(fill=fill, stroke=stroke, ink=ink, font_size=font, stroke_width=width)

    board = dict(version=1, width=WIDTH, height=HEIGHT, viewport=INITIAL_CAMERA, background='@diagram-canvas', styles={
        'frame': style('@diagram-normal-fill', '@diagram-normal-stroke', font=72),
        'hero': style(font=100), 'section': style(font=82, ink='@diagram-title-ink'),
        'body': style(), 'caption': style(font=60, ink='@diagram-normal-ink'),
        'flow': style(stroke='@diagram-yellow-accent', ink='@diagram-yellow-ink', font=60, width=7),
        'mapping': dict(**style(stroke='@diagram-blue-accent', font=60, width=5), dash=True),
        'anchor': style(), 'area': style(font=60, width=3),
    }, elements=[])

    def add(element):
        board['elements'].append(element)
        return element

    def text(key, label, x, y, w, h, source, style='body', parent=None):
        e = dict(id=key, kind='text', label=label, x=x, y=y, width=w, height=h,
                 style=style, wrap=True, source=ref(source))
        if parent:
            e['parent'] = parent
        return add(e)

    # Root-level backgrounds preserve every existing frame/element instance path.
    # Different extents follow narrative neighborhoods, not equal diagram tiles.
    sections = [
        ('delivery-area', 'Delivery', 'yellow', 3910, 60, 6030, 2720,
         'local-capture-delivery'),
        ('search-area', 'Search', 'blue', 40, 2780, 5390, 4640,
         'local-project-boundary'),
        ('authority-area', 'Authority', 'purple', 5580, 3080, 4360, 4570,
         'cloud-project-authority'),
    ]
    for key, label, color, x, y, width, height, source in sections:
        add(dict(id=key, kind='group', shape='section', label=label, color=color,
                 x=x, y=y, width=width, height=height, style='area', source=ref(source),
                 description=f'{label} reading neighborhood: related diagrams and narrative. '
                             'Color is a reading aid, not an additional system or authority boundary.'))

    text('title', 'DayLight / how work becomes shared evidence',
         160, 140, 3280, 150, 'local-cloud-overview', 'hero')
    text('intro', 'Local work and hosted search keep different records. Follow one source grant and generation across the stores, then explore delivery, search and authority.',
         160, 340, 3280, 260, 'local-cloud-overview')
    text('read-model', 'Start with the system data model', 160, 650, 1650, 110,
         'local-cloud-overview', 'section')

    # Ordinary groups are narrative anchors, not replacement cards for ERDs.
    # The actual cross-store drawing is immediately beside these three areas.
    narratives = [
        ('delivery', '01  Deliver the same work', 660,
         'Observation progress, destination debt and history ceilings stay separate. The server admits a delivery for an exact source generation.',
         'local-capture-delivery'),
        ('search', '02  Build search projections', 1190,
         'Local selections become search scope. In the cloud, immutable source bytes supply derived search. A file cursor is not a delivery receipt.',
         'local-cloud-overview'),
        ('authority', '03  Decide who may read', 1720,
         'Ownership and policies guard admission. Shared Project membership and exact-source consent separately govern access to hosted search.',
         'local-cloud-overview'),
    ]
    for key, title, y, body, source in narratives:
        add(dict(id=key+'-guide', kind='group', x=1980, y=y, width=1460,
                 height=460, style='anchor', source=ref(source), description=body))
        text(key+'-title', title, 60, 0, 1400, 115, source, 'section', key+'-guide')
        text(key+'-guide-story', body, 60, 135, 1400, 325, source, parent=key+'-guide')
    text('evidence-note', 'Committed-code documentation · select a diagram to inspect its pinned evidence',
         160, 2260, 3280, 100, 'local-cloud-overview', 'caption')

    text('delivery-heading', '01 / Delivery: progress is not acknowledgement',
         4050, 180, 5700, 140, 'local-capture-delivery', 'hero')
    text('delivery-story', 'The local model separates what was observed from what each destination is still owed. Admission preserves the exact source generation and accepted rules; immutable bytes then supply hosted search projections.',
         4050, 380, 5300, 280, 'local-cloud-overview')
    text('delivery-meaning', 'Amber connections follow delivery context. They do not add SQL foreign keys.',
         4050, 730, 5300, 100, 'local-cloud-overview', 'caption')
    text('delivery-caution', 'Only the payload-to-delivery line in the cross-store ERD is a physical FK. Grant tuples and generation mappings retain their own meaning.',
         4050, 2490, 5500, 240, 'local-cloud-overview')

    text('search-heading', '02 / Search: projections keep their own scope',
         160, 2880, 5050, 140, 'local-project-boundary', 'hero')
    text('search-story', 'Durable selections project into search membership across database files. File cursors record append progress; search documents carry retrieval state. Neither is a cloud delivery acknowledgement.',
         160, 3100, 4900, 260, 'local-index-progress')
    text('episodes-story', 'Episode catalogs rebuild from validated manifests. Episode targets and prompt bindings cross database boundaries; original files and manifests retain authority.',
         160, 4930, 4900, 260, 'local-episode-catalog')
    text('local-directory-note', '27 local SQL tables; the whole view draws only the nine declared physical FKs.',
         3610, 7030, 1700, 240, 'local-whole-model', 'caption')

    text('authority-heading', '03 / Authority: admission and access',
         5700, 3200, 4100, 140, 'cloud-project-authority', 'hero')
    text('authority-story', 'Storage Project ownership and immutable policies guard admission. Identity supplies principal and Organization evidence; current consent remains separate from migration evidence and legacy visibility.',
         5700, 3420, 4100, 340, 'cloud-project-authority')
    text('access-story', 'Shared destinations and people groups are distinct. OAuth binds a client and principal to one selected scope; tokens do not replace authority checks.',
         5700, 5740, 2800, 290, 'cloud-oauth')

    text('support-heading', 'Further context / directory, people and durable records',
         160, 7710, 6600, 140, 'cloud-enterprise', 'hero')
    text('provisioning-story', 'Provisioning maintains directory state, not transcript access.',
         160, 7930, 2800, 200, 'cloud-scim')
    text('message-story', 'Team-scoped messages are separate from captured prompt evidence.',
         3260, 7930, 1400, 200, 'cloud-messaging')
    text('cloud-directory-note', '49 cloud tables by ownership domain. Colors aid navigation; they do not claim deployed SQL role separation.',
         6700, 9630, 3000, 280, 'cloud-overview', 'caption')
    text('return-story', 'Return to the system model', 7800, 7660, 1900, 140,
         'local-cloud-overview', 'section')

    frames = {}
    def frame(instance, key, label, x, y, width):
        _, sw, sh = images[key]
        height = width*sh/sw
        frames[instance] = dict(x=x, y=y, width=width, height=height)
        add(dict(id=instance, kind='frame', label=label,
                 description=records[key]['explanation'], x=x, y=y, width=width,
                 height=height, style='frame', detail_scale=.6, source=ref(key)))

    for key, _, label, x, y, width in AREAS:
        frame(key, key, label, x, y, width)
    # Same pin, a distinct appearance: navigation never creates another source.
    frame('system-return', 'local-cloud-overview', 'The same cross-store model', 7800, 8030, 1750)

    def connect(key, start, end, source, label, description, box, style='mapping', bends=()):
        add(dict(id=key, kind='connection', x=0, y=0, **{'from': start, 'to': end},
                 label=label, label_box=dict(zip(('x', 'y', 'width', 'height'), box)),
                 style=style, head='arrow', wrap=True, description=description,
                 source=ref(source), bends=[dict(x=x, y=y) for x, y in bends]))

    # Short labeled guides establish the reading path. These describe views,
    # not a newly asserted processing stage or a recorded database relationship.
    connect('read-delivery', 'local-cloud-overview', 'delivery-guide', 'local-cloud-overview',
            'delivery', 'Read the grant, generation and admitted delivery in the cross-store model; continue right for focused ERDs.',
            (1725, 1010, 310, 100), bends=[(1890, 1120), (1890, 900)])
    connect('read-search', 'local-cloud-overview', 'search-guide', 'local-cloud-overview',
            'search', 'Compare independent local search with hosted projections; continue below for the local models.',
            (1725, 1450, 310, 100), bends=[(1890, 1560), (1890, 1450)])
    connect('read-authority', 'local-cloud-overview', 'authority-guide', 'local-cloud-overview',
            'access', 'Shared Project membership and exact-source consent separately govern hosted access; inspect authority models farther right and below.',
            (1725, 1930, 310, 100), bends=[(1890, 2040), (1890, 1940)])
    connect('admitted-generation', 'local-capture-delivery', 'cloud-source-lineage', 'local-cloud-overview',
            'exact source / generation', 'Explanatory delivery context: the same source grant and generation reaches server admission. This is not a physical SQL FK.',
            (5560, 1400, 480, 240), 'flow')
    connect('hosted-projections', 'cloud-source-lineage', 'cloud-corpus', 'local-cloud-overview',
            'admitted delivery', 'The cross-store model relates admitted delivery to object storage and hosted search projections. This is not an additional SQL FK.',
            (7560, 1400, 480, 240), 'flow')
    connect('search-membership', 'local-project-boundary', 'local-search-retrieval', 'local-project-boundary',
            'selections project into membership', 'Durable selections project into search membership across files, with no cross-file SQL FK.',
            (1100, 3490, 2300, 110), bends=[(790, 3460), (3960, 3460)])

    by_id = {e['id']: e for e in board['elements']}
    assert len(by_id) == len(board['elements']), 'duplicate element IDs'
    def bounds(e):
        x, y = e['x'], e['y']
        parent = e.get('parent')
        while parent:
            p = by_id[parent]
            x, y = x+p['x'], y+p['y']
            parent = p.get('parent')
        return x, y, e.get('width', 0), e.get('height', 0)

    def overlaps(a, b):
        x, y, w, h = a
        u, v, c, d = b
        return not (x+w <= u or u+c <= x or y+h <= v or v+d <= y)

    # Reserve the complete child rectangle and a separate title band.
    boxes = [(key, (f['x'], f['y']-CAPTION_CLEARANCE, f['width'], f['height']+CAPTION_CLEARANCE))
             for key, f in frames.items()]
    texts = [e for e in board['elements'] if e['kind'] == 'text']
    for i, (key, box) in enumerate(boxes):
        x, y, w, h = box
        assert 0 <= x and 0 <= y and x+w <= WIDTH and y+h <= HEIGHT, key
        for other, other_box in boxes[i+1:]:
            assert not overlaps(box, other_box), f'frame/caption overlap: {key}, {other}'
        for e in texts:
            assert not overlaps(box, bounds(e)), f'callout overlap: {key}, {e["id"]}'
    for i, e in enumerate(texts):
        x, y, w, h = bounds(e)
        assert 0 <= x and 0 <= y and x+w <= WIDTH and y+h <= HEIGHT, e['id']
        for other in texts[i+1:]:
            assert not overlaps(bounds(e), bounds(other)), f'text overlap: {e["id"]}, {other["id"]}'

    # Section tabs reserve 100 units above their contents; side gutters are at
    # least 100 units and bottom gutters at least 50 units. Include complete
    # diagram caption bands in the check.
    section_boxes = [(key, (x, y, w, h)) for key, _, _, x, y, w, h, _ in sections]
    for i, (key, box) in enumerate(section_boxes):
        x, y, w, h = box
        assert 0 <= x and 0 <= y and x+w <= WIDTH and y+h <= HEIGHT, key
        for other, other_box in section_boxes[i+1:]:
            assert not overlaps(box, other_box), f'section overlap: {key}, {other}'
        for item, child in boxes + [(e['id'], bounds(e)) for e in texts]:
            if overlaps(box, child):
                u, v, c, d = child
                assert x+100 <= u and u+c <= x+w-100, f'section side gutter: {key}, {item}'
                assert y+100 <= v and v+d <= y+h-50, f'section title/bottom gutter: {key}, {item}'

    camera = dict(initial=INITIAL_CAMERA, canvas=dict(width=WIDTH, height=HEIGHT),
                  note='Mirrors the authored document viewport. Fit-all is an optional atlas.',
                  tours=[dict(name='Delivery', anchor='delivery-heading', x=3900, y=0, width=6100, height=2900),
                         dict(name='Local search', anchor='search-heading', x=0, y=2800, width=5400, height=4600),
                         dict(name='Authority and access', anchor='authority-heading', x=5500, y=3000, width=4500, height=4500)])
    for path, digest in source_reads.items():
        assert 'sha256:' + hashlib.sha256(path.read_bytes()).hexdigest() == digest, f'source changed: {path}'
    audit = dict(verified_source_files=len(source_reads), unique_pins=len(manifest), frame_instances=len(frames),
                 repeated_instances={'local-cloud-overview': ['local-cloud-overview', 'system-return']},
                 all_visual_digests_verified=True, source_modified=False, published=False,
                 evidence='Pinned committed-code documentation, not a live database inspection.',
                 structured_sources=sum('diagram' in r for r in records.values()),
                 svg_only_sources=sum('diagram' not in r for r in records.values()),
                 source_pins='source-pins.json',
                 sections=[dict(id=key, label=label, color=color)
                           for key, label, color, *_ in sections])
    for name, data in [('daylight-board.json', board), ('source-pins.json', manifest),
                       ('camera-proposal.json', camera), ('source-audit.json', audit)]:
        (output/name).write_text(json.dumps(data, indent=2)+'\n')

    # A preview must use the same renderer as publication, including section
    # palette tokens, title tabs, wrapping and composition. Do not approximate it.
    preview = None
    if args.rendered_svg:
        rendered = args.rendered_svg.read_text()
        svg_root = ET.fromstring(rendered)
        expected_viewbox = f'0 0 {WIDTH} {HEIGHT}'
        if svg_root.attrib.get('viewBox') != expected_viewbox:
            raise ValueError('rendered SVG must have the authored full-canvas viewBox')
        (output/'layout-preview.svg').write_text(rendered)
        initial_svg = rendered.replace(f'viewBox="{expected_viewbox}"',
                                       'viewBox="0 0 3600 2400"', 1)
        (output/'initial-preview.svg').write_text(initial_svg)
        preview = output/'layout-preview.html'
        preview.write_text('<!doctype html><meta charset="utf-8"><title>DayLight section layout proof</title><style>body{margin:0;background:#f7f8f6;font:16px Arial}nav{position:absolute;right:16px;top:16px}button{padding:8px}img{width:100vw;height:100vh;object-fit:contain}</style><nav><button onclick="document.querySelector(\'img\').src=\'initial-preview.svg\'">Start</button> <button onclick="document.querySelector(\'img\').src=\'layout-preview.svg\'">Fit atlas</button></nav><img src="initial-preview.svg" alt="DayLight system ERD with labeled Delivery, Search and Authority sections">')
    print(json.dumps({'board': str(output/'daylight-board.json'), 'frames': len(frames),
                      'pins': str(output/'source-pins.json'), 'audit': str(output/'source-audit.json'),
                      'camera': str(output/'camera-proposal.json'),
                      'preview': str(preview) if preview else None}, indent=2))


if __name__ == '__main__':
    main()
