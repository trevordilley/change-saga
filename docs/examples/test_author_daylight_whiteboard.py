#!/usr/bin/env python3
"""Selected narrative/evidence regressions; opt in with DAYLIGHT_SAGA."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

SCRIPT = Path(__file__).with_name('author-daylight-whiteboard.py')
SPEC = importlib.util.spec_from_file_location('daylight_author', SCRIPT)
AUTHOR = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(AUTHOR)


class RouteGeometryTest(unittest.TestCase):
    def test_clearance_checks_detect_crossings_and_clip_endpoints(self):
        rect = (10, 10, 100, 100)
        for a, b in [((0, 60), (120, 60)), ((60, 0), (60, 120)), ((40, 40), (50, 50))]:
            self.assertTrue(AUTHOR.segment_hits(a, b, rect))
        self.assertFalse(AUTHOR.segment_hits((0, 10), (120, 10), rect))
        self.assertEqual(AUTHOR.boundary(rect, (200, 60)), (110, 60))
        self.assertFalse(AUTHOR.overlaps(rect, (110, 10, 20, 20)))


@unittest.skipUnless(os.environ.get('DAYLIGHT_SAGA'), 'set DAYLIGHT_SAGA for pinned source regression')
class NarrativeTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.root = Path(os.environ['DAYLIGHT_SAGA']).resolve()
        cls.tmp = tempfile.TemporaryDirectory()
        cls.addClassCleanup(cls.tmp.cleanup)
        cls.out = Path(cls.tmp.name)
        cls.before = {p: AUTHOR.file_digest(p) for p in cls.root.rglob('*') if p.is_file()}
        subprocess.run([sys.executable, str(SCRIPT), str(cls.root), str(cls.out)], check=True, stdout=subprocess.DEVNULL)
        cls.board = json.loads((cls.out/'daylight-board.json').read_text())
        cls.audit = json.loads((cls.out/'source-audit.json').read_text())
        cls.evidence = json.loads((cls.out/'entity-evidence.json').read_text())
        cls.connections = json.loads((cls.out/'connection-evidence.json').read_text())
        cls.elements = {e['id']: e for e in cls.board['elements']}
        cls.documentation = {e['id']: e for e in cls.audit['evidence']}

    def source(self, pin):
        parts = pin['target'].split(':')
        kind, key = parts[-2:]
        revision = pin['revision'].split(':')[-1]
        folder = 'data-entities' if kind == 'data-entity' else 'erds'
        return json.loads((self.root/'___inventory'/folder/(key+'.'+kind)/'revisions'/(revision+'.json')).read_text())

    def path(self, e):
        return (self.path(self.elements[e['parent']])+'/' if e.get('parent') else '')+e['id']

    def bounds(self, e):
        x, y = e['x'], e['y']
        parent = e.get('parent')
        while parent:
            p = self.elements[parent]
            x, y = x+p['x'], y+p['y']
            parent = p.get('parent')
        return x, y, e['width'], e['height']

    def test_every_depicted_scope_retains_exact_source_records(self):
        pinned = {}
        for key, revision in AUTHOR.PINS:
            r = self.source(dict(target='urn:change-saga:daylight:erd:'+key, revision='revision:'+revision))
            for pin in r['directory']:
                pinned[pin['target']] = pin
        selected = [d for d in self.documentation.values() if d['kind'] == 'entity']
        self.assertEqual({d['id'] for d in selected}, set(AUTHOR.SELECTED_ENTITIES))
        for d in selected:
            e = self.elements[d['id']]
            self.assertEqual(e['source'], pinned[e['source']['target']])
            self.assertEqual(e['source']['target'].split(':')[-1], AUTHOR.SELECTED_ENTITIES[e['id']])
            self.assertEqual(d['evidence_pins'], [e['source']])
            self.assertIn(self.source(e['source'])['explanation'], e['description'])
        for item in self.evidence:
            self.assertEqual(item['record'], self.source(item['source']))
        retained = {item['source']['target'] for item in self.evidence}
        for d in self.documentation.values():
            self.assertIn(d['source']['target'], retained)
            self.assertTrue(d['scope'])
            for pin in d['evidence_pins']:
                self.assertIn(pin['target'], retained)
                self.assertEqual(self.source(pin)['id'], pin['revision'].split(':')[-1])

    def test_crowfeet_match_only_selected_authentic_associations(self):
        expected = {('local-pending-project-deliveries', 'fk-observationid'),
                    ('cloud-prompt', 'logical-admission'), ('cloud-projects', 'fk-2'),
                    ('cloud-projects', 'fk-policy-parent'), ('local-episode-artifacts', 'fk-episode-id')}
        actual = set()
        for item in self.connections:
            edge = self.elements[item['id']]
            if not item['recorded']:
                self.assertEqual(edge['head'], 'arrow')
                self.assertNotIn('tail', edge)
                self.assertNotIn('relationship', edge['source'])
                continue
            pin = item['source']
            actual.add((pin['target'].split(':')[-1], pin['relationship']))
            relation = next(r for r in self.source(pin)['relationships'] if r['id'] == pin['relationship'])
            self.assertEqual(item['relation'], relation)  # Includes exact code evidence and explanation.
            self.assertEqual(edge['tail'], AUTHOR.MARKERS[relation['cardinality']['owner']])
            self.assertEqual(edge['head'], AUTHOR.MARKERS[relation['cardinality']['destination']])
            owner = self.elements[edge['from'].split('/')[-1]]
            destination = self.elements[edge['to'].split('/')[-1]]
            self.assertEqual(owner['source'], {k: pin[k] for k in ('target', 'revision')})
            self.assertEqual(destination['source'], relation['destination'])
            self.assertEqual(edge['style'], 'fk' if relation['explanation'].lower().startswith('physical') else 'application')
        self.assertEqual(actual, expected)
        self.assertEqual((self.elements['policy-parent']['tail'], self.elements['policy-parent']['head']), ('only-one', 'zero-or-many'))
        self.assertEqual((self.elements['current-policy']['tail'], self.elements['current-policy']['head']), ('zero-or-one', 'only-one'))

    def test_aggregate_concepts_and_refusal_do_not_impersonate_tables(self):
        for key, members in {
            'search-metadata': {'local-source-files', 'local-search-documents'},
            'membership': {'cloud-teams', 'cloud-team-members'},
            'recording-files': {'local-episodes', 'local-episode-artifacts'},
        }.items():
            d = self.documentation[key]
            self.assertEqual(d['kind'], 'concept')
            self.assertEqual({p['target'].split(':')[-1] for p in d['evidence_pins']}, members)
            self.assertIn(':erd:', self.elements[key]['source']['target'])
            self.assertIn('not', d['scope'])
            self.assertIn('broader pinned ERD scope', self.elements[key]['note'])
        self.assertIn('no plaintext fallback', self.elements['search-unavailable']['description'])
        self.assertTrue(self.board['styles']['refusal']['dash'])
        self.assertEqual(self.elements['capability-failure']['style'], 'failure-flow')
        self.assertIn('BOTH checks', self.elements['shared-read']['label'])
        self.assertEqual({self.elements[k]['from'].split('/')[-1] for k in ['membership-check', 'consent-check']}, {'membership', 'source-consent'})
        self.assertEqual({self.elements[k]['to'].split('/')[-1] for k in ['membership-check', 'consent-check']}, {'shared-read'})
        self.assertIn('application lifecycle', self.elements['deletion-callout']['label'])
        self.assertIn('not a database FK', self.elements['body-source']['description'])
        self.assertIn('every watched file or search result', self.elements['transcript-capture']['description'])
        self.assertIn('not a direct FK', self.elements['episode-context']['description'])
        self.assertIn('Arrowheads: conceptual', self.elements['legend']['label'])

    def test_four_readable_areas_have_real_members_and_incident_connections(self):
        roots = {e['id'] for e in self.elements.values() if e['kind'] == 'group' and not e.get('parent')}
        self.assertEqual(roots, {'search-area', 'delivery-area', 'authority-area', 'episodes-area'})
        seen = set()
        for area in self.audit['areas']:
            root = self.elements[area['id']]
            self.assertEqual(self.path(root), area['path'])
            members = {e['id'] for e in self.elements.values() if self.path(e).startswith(area['path']+'/')}
            self.assertTrue(any(self.elements[k]['kind'] == 'node' for k in members))
            seen.update(members)
            self.assertTrue(any(c['from'].split('/')[-1] in members or c['to'].split('/')[-1] in members
                                for c in self.elements.values() if c['kind'] == 'connection'))
            # 1140 x 900 desktop canvas; reserve more space for title/toolbar and focus padding.
            scale = min(1100/(root['width']+48), 780/(root['height']+48))
            for k in members:
                e = self.elements[k]
                if e.get('label'):
                    self.assertGreaterEqual(self.board['styles'][e['style']]['font_size']*scale, 16, k)
                x, y, w, h = self.bounds(e)
                rx, ry, rw, rh = self.bounds(root)
                self.assertTrue(rx <= x and ry <= y and x+w <= rx+rw and y+h <= ry+rh, k)
        # The four stories belong to one connected model, with scoped arrows
        # between areas rather than only a prose reading-order suggestion.
        graph = {key: set() for key in roots}
        for edge in self.elements.values():
            if edge['kind'] != 'connection':
                continue
            a, b = edge['from'].split('/')[0], edge['to'].split('/')[0]
            if a != b:
                graph[a].add(b)
                graph[b].add(a)
        reached, pending = set(), ['search-area']
        while pending:
            key = pending.pop()
            if key not in reached:
                reached.add(key)
                pending.extend(graph[key]-reached)
        self.assertEqual(reached, roots)
        self.assertTrue(set(AUTHOR.SELECTED_ENTITIES) <= seen)
        opening_scale = min(1100/self.board['viewport']['width'], 780/self.board['viewport']['height'])
        self.assertGreaterEqual(self.board['styles']['text']['font_size']*opening_scale, 16)
        self.assertIn('Files retain truth', self.elements['board-thesis']['label'])
        self.assertIn('04 Keep recording truth', self.elements['route-map']['label'])
        self.assertTrue(all(not e.get('decorative') for e in self.elements.values()))

    def test_selection_has_no_hidden_inventory_or_imported_visual(self):
        self.assertLess(len(self.elements), 90)
        self.assertEqual(len(AUTHOR.SELECTED_ENTITIES), 10)
        self.assertTrue(all(e['kind'] not in ('frame', 'graphic') for e in self.elements.values()))
        self.assertTrue(all('visibility' not in e for e in self.elements.values()))
        # Fixed-size native detail/field text is deliberately not used for body prose.
        self.assertTrue(all('detail' not in e and 'fields' not in e for e in self.elements.values()))
        self.assertEqual({e['id'] for e in self.elements.values()}, set(self.documentation) | {c['id'] for c in self.connections if c['recorded']})
        self.assertIn('intentionally not inventory coverage', self.audit['mode'])

    def test_source_is_unchanged_and_source_svg_is_never_read(self):
        self.assertEqual(self.before, {p: AUTHOR.file_digest(p) for p in self.root.rglob('*') if p.is_file()})
        read_bytes, read_text = Path.read_bytes, Path.read_text
        def guard_bytes(path, *args, **kwargs):
            self.assertNotEqual(path.suffix, '.svg')
            return read_bytes(path, *args, **kwargs)
        def guard_text(path, *args, **kwargs):
            self.assertNotEqual(path.suffix, '.svg')
            return read_text(path, *args, **kwargs)
        with tempfile.TemporaryDirectory() as out:
            with mock.patch.object(Path, 'read_bytes', guard_bytes), mock.patch.object(Path, 'read_text', guard_text), \
                    mock.patch.object(sys, 'argv', [str(SCRIPT), str(self.root), out]), mock.patch('builtins.print'):
                AUTHOR.main()
            self.assertEqual((Path(out)/'daylight-board.json').read_bytes(), (self.out/'daylight-board.json').read_bytes())

    def test_output_must_be_outside_source(self):
        result = subprocess.run([sys.executable, str(SCRIPT), str(self.root), str(self.root/'forbidden-output')], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('output must be outside', result.stderr)
        self.assertFalse((self.root/'forbidden-output').exists())


if __name__ == '__main__':
    unittest.main()
