#!/usr/bin/env python3
"""Author a pinned DayLight composition from a read-only local saga copy.

Python standard library only. Never publishes or modifies the input saga.
Exact revision IDs below are intentional: this example must not follow heads.
"""
import argparse
import base64
import hashlib
import html
import json
from pathlib import Path
import textwrap
import xml.etree.ElementTree as ET

# Explicit composition, not an auto-layout or an index grid. Positions are in
# design units, scaled uniformly to stay inside the grammar's 10000-unit limit.
# id, revision, title, summary, x, y, width
AREAS = [
    ('local-project-boundary', 'r5-viewer-safe', 'Local Projects', 'Selections become search scope', 220, 1130, 1250),
    ('local-capture-delivery', 'r5-viewer-safe', 'Capture & delivery', 'Observation and destination debt', 1950, 1150, 1800),
    ('local-cloud-overview', 'r2', 'Across the stores', 'Grant, generation, admitted delivery', 4250, 1060, 2000),
    ('cloud-source-lineage', 'r5', 'Source admission', 'Exact generation and accepted rules', 6800, 1290, 1600),
    ('cloud-corpus', 'r4', 'Cloud corpus', 'Immutable bytes; derived search', 9050, 1430, 1650),
    ('local-index-progress', 'r5-viewer-safe', 'Index progress', 'File cursors track append progress', 240, 2560, 1150),
    ('local-search-retrieval', 'r5-viewer-safe', 'Local search', 'Sources, documents and scope', 1850, 2950, 1800),
    ('local-episode-catalog', 'r5-viewer-safe', 'Episode catalog', 'Rebuilt from validated manifests', 260, 3820, 1200),
    ('local-episode-correlation', 'r5-viewer-safe', 'Episode links', 'Projections across databases', 1900, 4690, 1650),
    ('local-whole-model', 'r5-viewer-safe', 'Whole local model', '27 tables; nine physical FKs', 300, 5840, 1550),
    ('cloud-project-authority', 'r5', 'Project authority', 'Ownership and immutable policies', 4350, 3410, 1550),
    ('cloud-identity', 'r4', 'Identity & tenancy', 'Principals and Organization evidence', 6350, 3410, 1550),
    ('cloud-contribution', 'r4', 'Sharing rules', 'Current consent; legacy evidence', 8340, 3270, 1150),
    ('cloud-shared-destinations', 'r4', 'Projects & Teams', 'Shared destinations and people groups', 9950, 3560, 1150),
    ('cloud-enterprise', 'r4', 'Enterprise & audit', 'Identity mappings; surviving audit', 4210, 5750, 1100),
    ('cloud-scim', 'r4', 'SCIM directory', 'Provisioning resources and tokens', 5530, 6270, 1050),
    ('cloud-oauth', 'r4', 'MCP grants', 'Client, principal, selected scope', 6920, 5650, 1050),
    ('cloud-invitations', 'r4', 'Invitations', 'Eligibility and redemption receipts', 8420, 5000, 1080),
    ('cloud-messaging', 'r5', 'Durable messages', 'Team-scoped person-to-person records', 9950, 5430, 1300),
    ('cloud-overview', 'r3', 'Whole cloud model', '49 tables by ownership domain', 9770, 6740, 1400),
]
SCALE = .8
WIDTH, HEIGHT = 11700, 8250
CAPTION_CLEARANCE = 240  # Three 64-unit lines plus padding, above every SVG.


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('saga', type=Path, help='read-only DayLight saga copy')
    parser.add_argument('output', type=Path, help='new output directory outside the saga')
    args = parser.parse_args()
    root, output = args.saga.resolve(), args.output.resolve()
    if output == root or root in output.parents:
        parser.error('output must be outside the source saga')
    output.mkdir(parents=True, exist_ok=True)
    records, images, manifest = {}, {}, []
    for key, revision, *_ in AREAS:
        directory = root / '___inventory' / 'erds' / (key + '.erd')
        matches = [json.loads(p.read_text()) for p in sorted((directory / 'revisions').glob('*.json'))]
        matches = [r for r in matches if r['id'] == revision]
        if len(matches) != 1:
            raise ValueError(f'{key}: expected exact revision {revision}')
        record = records[key] = matches[0]
        asset = (directory / record['visual']['path']).resolve()
        if root not in asset.parents:
            raise ValueError(f'{key}: asset escapes source saga')
        svg = asset.read_bytes()
        digest = 'sha256:' + hashlib.sha256(svg).hexdigest()
        if digest != record['visual']['digest']:
            raise ValueError(f'{key}: source SVG digest mismatch')
        viewbox = [float(n) for n in ET.fromstring(svg).attrib['viewBox'].split()]
        images[key] = (svg, viewbox[2], viewbox[3])
        manifest.append({'id': key, 'target': record['record'],
                         'revision': record['record'] + ':revision:' + revision,
                         'visual_digest': digest, 'intrinsic_width': viewbox[2],
                         'intrinsic_height': viewbox[3], 'explanation': record['explanation']})

    def ref(key):
        r = records[key]
        return {'target': r['record'], 'revision': r['record'] + ':revision:' + r['id']}

    def style(fill='none', stroke='none', ink='#243746', font=64, width=3):
        return dict(fill=fill, stroke=stroke, ink=ink, font_size=font*SCALE, stroke_width=width*SCALE)

    board = dict(version=1, width=WIDTH*SCALE, height=HEIGHT*SCALE, background='#f7f8f6', styles={
        'frame': style('#ffffff', '#cad3d7'),
        'hero': style(font=150), 'section': style(font=106, ink='#355b64'),
        'callout': style(font=82), 'caption': style(font=66, ink='#526776'),
        'flow': style(stroke='#a46f28', ink='#80521b', font=60, width=9),
        'mapping': dict(**style(stroke='#587d87', font=60, width=7), dash=True),
    }, elements=[])

    def add(element):
        for key in ('x', 'y', 'width', 'height'):
            if key in element:
                element[key] = round(element[key]*SCALE, 6)
        for key in ('target', 'label_box'):
            if key in element:
                element[key] = {k: round(v*SCALE, 6) for k, v in element[key].items()}
        if 'bends' in element:
            element['bends'] = [{k: round(v*SCALE, 6) for k, v in point.items()} for point in element['bends']]
        board['elements'].append(element)

    def text(key, label, x, y, w, h, source, style='callout'):
        add(dict(id=key, kind='text', label=label, x=x, y=y, width=w, height=h,
                 style=style, wrap=True, source=ref(source)))

    text('title', 'DayLight / from local work to shared evidence', 220, 100, 11200, 210, 'local-cloud-overview', 'hero')
    text('intro', 'One source grant and generation cross the store boundary. Search, consent and authority retain their own models.',
         230, 330, 10400, 250, 'local-cloud-overview')
    text('delivery-heading', '01   Follow the delivery', 1950, 640, 5600, 150, 'local-cloud-overview', 'section')
    text('local-heading', '02   Local search', 230, 2080, 1400, 160, 'local-cloud-overview', 'section')
    text('authority-heading', '03   Authority & consent', 4350, 2920, 3700, 160, 'cloud-project-authority', 'section')
    text('local-callout', 'A file cursor is not a cloud delivery acknowledgement.', 1900, 2420, 1850, 290, 'local-index-progress')
    text('episode-callout', 'Original files and episode manifests remain outside SQL authority.',
         250, 4910, 1400, 610, 'local-whole-model')
    text('scope-callout', 'Shared Project membership and exact-source consent separately govern hosted search.',
         8340, 2790, 3000, 230, 'local-cloud-overview')
    text('directory-callout', 'Provisioning maintains directory state. It does not grant transcript access.',
         4250, 7210, 3750, 420, 'cloud-scim')
    text('cloud-directory-callout', 'The whole-model directory groups ownership domains; focused ERDs carry the relationship lines.',
         8360, 6880, 1120, 920, 'cloud-overview', 'caption')
    text('source-note', 'Pinned committed-code documentation / focus any diagram in place to inspect its original source.',
         230, 7840, 8950, 180, 'local-cloud-overview', 'caption')

    frames = {}
    for key, _, label, summary, x, y, width in AREAS:
        _, sw, sh = images[key]
        height = width*sh/sw
        frames[key] = dict(x=x, y=y, width=width, height=height)
        add(dict(id=key, kind='frame', label=label, summary=summary,
                 description=records[key]['explanation'], x=x, y=y, width=width,
                 height=height, style='frame', detail_scale=.6, source=ref(key)))

    # These connect explanatory appearances, not newly asserted SQL FKs.
    def connect(key, start, end, source, description, style='mapping', bends=None):
        element = dict(id=key, kind='connection', x=0, y=0, **{'from': start, 'to': end},
                       style=style, head='arrow', description=description, source=ref(source))
        if bends:
            element['bends'] = [dict(x=x, y=y) for x, y in bends]
        add(element)

    connect('selection-search-scope', 'local-project-boundary', 'local-search-retrieval', 'local-project-boundary',
            'Durable source selections project into search membership across files. This explanatory mapping is not a physical SQL foreign key.',
            bends=[(1650, 2080), (1650, 3380)])
    connect('capture-in-context', 'local-capture-delivery', 'local-cloud-overview', 'local-cloud-overview',
            'Read the capture grant and generation in the cross-store composite. The composite is another view of the same model, not another processing stage.')
    connect('admission-in-context', 'local-cloud-overview', 'cloud-source-lineage', 'local-cloud-overview',
            'The cross-store overview follows the same source grant and generation to a delivery admitted by the server. This line relates two views of that admission.', 'flow')
    connect('hosted-search-context', 'cloud-source-lineage', 'cloud-corpus', 'local-cloud-overview',
            'The cross-store overview connects admitted delivery with object storage and hosted search projections. This is an explanatory data-flow connection, not a SQL foreign key.', 'flow')

    connect('policies-before-admission', 'cloud-project-authority', 'cloud-source-lineage', 'cloud-project-authority',
            'Direct ownership and immutable policies guard the storage Project before capture admission. This contextual connection does not assert an additional SQL relationship.',
            bends=[(6150, 3280), (6500, 2770)])

    # Caption bands are real authored whitespace, never subtracted from frames.
    boxes = [(key, (f['x'], f['y']-CAPTION_CLEARANCE, f['width'], f['height']+CAPTION_CLEARANCE)) for key, f in frames.items()]
    for i, (key, (x, y, w, h)) in enumerate(boxes):
        assert 0 <= x and 0 <= y and x+w <= WIDTH and y+h <= HEIGHT, key
        for other, (a, b, c, d) in boxes[i+1:]:
            assert x+w <= a or a+c <= x or y+h <= b or b+d <= y, f'frame/caption overlap: {key}, {other}'
        for e in board['elements']:
            if e['kind'] != 'text':
                continue
            a, b, c, d = (e[k]/SCALE for k in ('x', 'y', 'width', 'height'))
            assert x+w <= a or a+c <= x or y+h <= b or b+d <= y, f'callout overlap: {key}, {e["id"]}'

    (output/'daylight-board.json').write_text(json.dumps(board, indent=2)+'\n')
    (output/'source-pins.json').write_text(json.dumps(manifest, indent=2)+'\n')
    # Diagnostic layout proof: original SVG bytes embedded at exact frame ratios.
    # This is intentionally not a substitute for the real viewer/browser checks.
    parts = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {WIDTH} {HEIGHT}" style="background:#f7f8f6;font-family:Arial,sans-serif">']
    for e in board['elements']:
        if e['kind'] == 'text':
            font = board['styles'][e['style']]['font_size']/SCALE
            x, y, w = (e[k]/SCALE for k in ('x', 'y', 'width'))
            for i, line in enumerate(textwrap.wrap(e['label'], max(1, int(w/(font*.53))))):
                parts.append(f'<text x="{x}" y="{y+font+i*font*1.2}" font-size="{font}" fill="#243746">{html.escape(line)}</text>')
    # Draw whole-frame connector routes for this diagnostic proof. Production
    # rendering still owns exact endpoint and marker behavior.
    def endpoint(frame, toward):
        cx, cy = frame['x']+frame['width']/2, frame['y']+frame['height']/2
        dx, dy = toward[0]-cx, toward[1]-cy
        t = min(frame['width']/(2*abs(dx)) if dx else float('inf'),
                frame['height']/(2*abs(dy)) if dy else float('inf'))
        return (cx+t*dx, cy+t*dy)

    for e in board['elements']:
        if e['kind'] != 'connection':
            continue
        a, b = frames[e['from']], frames[e['to']]
        bends = [(p['x']/SCALE, p['y']/SCALE) for p in e.get('bends', [])]
        ac, bc = (a['x']+a['width']/2, a['y']+a['height']/2), (b['x']+b['width']/2, b['y']+b['height']/2)
        points = [endpoint(a, bends[0] if bends else bc), *bends, endpoint(b, bends[-1] if bends else ac)]
        coords = ' '.join(f'{x},{y}' for x, y in points)
        color = board['styles'][e['style']]['stroke']
        dash = ' stroke-dasharray="22 18"' if e['style'] == 'mapping' else ''
        parts.append(f'<polyline points="{coords}" fill="none" stroke="{color}" stroke-width="8"{dash}/>')
    for key, _, label, summary, x, y, width in AREAS:
        svg, _, _ = images[key]
        h = frames[key]['height']
        parts.append(f'<image x="{x}" y="{y}" width="{width}" height="{h}" href="data:image/svg+xml;base64,{base64.b64encode(svg).decode()}"/>')
        parts.append(f'<text x="{x}" y="{y-164}" font-size="64" font-weight="bold" fill="#243746">{html.escape(label)}</text>')
        for i, line in enumerate(textwrap.wrap(summary, max(1, int(width/34)))):
            parts.append(f'<text x="{x}" y="{y-86+i*72}" font-size="64" fill="#526776">{html.escape(line)}</text>')
    parts.append('</svg>')
    (output/'layout-preview.svg').write_text('\n'.join(parts))
    (output/'layout-preview.html').write_text('<!doctype html><meta charset="utf-8"><title>DayLight composition layout proof</title><style>body{margin:0;background:#f7f8f6}img{width:100vw;height:100vh;object-fit:contain}</style><img src="layout-preview.svg" alt="DayLight composition with all twenty source ERDs">')
    print(json.dumps({'board': str(output/'daylight-board.json'), 'frames': len(frames),
                      'pins': str(output/'source-pins.json'), 'preview': str(output/'layout-preview.html'),
                      'caption_clearance': CAPTION_CLEARANCE*SCALE}, indent=2))


if __name__ == '__main__':
    main()
