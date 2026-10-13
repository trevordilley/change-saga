#!/usr/bin/env python3
"""Focused regression checks; real-source checks opt in with DAYLIGHT_SAGA.

DAYLIGHT_SAGA=/path/to/copied/daylight.saga python3 -m unittest discover \
    -s docs/examples -p 'test_author_daylight_whiteboard.py'
"""
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
    def test_crossing_and_containment_are_detected(self):
        rect = (10, 10, 100, 100)
        for start, end in [((0,60),(120,60)), ((60,0),(60,120)),
                           ((0,0),(120,120)), ((40,40),(50,50))]:
            self.assertTrue(AUTHOR.segment_hits(start, end, rect))
        for start, end in [((0,10),(120,10)), ((0,0),(5,5)),
                           ((120,0),(120,130))]:
            self.assertFalse(AUTHOR.segment_hits(start, end, rect))

    def test_ray_clips_to_bounds_without_invented_ports(self):
        self.assertEqual(AUTHOR.boundary((10,10,100,100),(200,60)), (110,60))
        self.assertEqual(AUTHOR.boundary((10,10,100,100),(60,0)), (60,10))
        self.assertEqual(AUTHOR.boundary((10,10,100,100),(60,60)), (60,60))


@unittest.skipUnless(os.environ.get('DAYLIGHT_SAGA'), 'set DAYLIGHT_SAGA for pinned real-data regression')
class PinnedCompositionTest(unittest.TestCase):
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
        cls.evidence = json.loads((cls.out/'connection-evidence.json').read_text())
        cls.entity_evidence = json.loads((cls.out/'entity-evidence.json').read_text())
        cls.elements = {e['id']: e for e in cls.board['elements']}
        cls.pinned_entities = {}
        for key, revision, _ in AUTHOR.PINS:
            path = cls.root/'___inventory'/'erds'/(key+'.erd')/'revisions'/(revision+'.json')
            for pin in json.loads(path.read_text())['directory']:
                cls.pinned_entities[pin['target']] = pin

    def source(self, pin):
        key, revision = pin['target'].split(':')[-1], pin['revision'].split(':')[-1]
        return json.loads((self.root/'___inventory'/'data-entities'/(key+'.data-entity')/'revisions'/(revision+'.json')).read_text())

    def test_source_tree_is_unchanged_and_every_pin_remains(self):
        self.assertEqual(self.before, {p: AUTHOR.file_digest(p) for p in self.root.rglob('*') if p.is_file()})
        frames = [e for e in self.elements.values() if e['kind']=='frame']
        self.assertEqual(frames, [])
        pins = json.loads((self.out/'source-pins.json').read_text())
        self.assertEqual({(p['id'], p['revision'].split(':')[-1]) for p in pins},
                         {(key, rev) for key, rev, _ in AUTHOR.PINS})
        self.assertEqual(self.audit['covered_entities'], 76)
        self.assertEqual(self.audit['primary_native_entities'], 76)
        self.assertEqual(self.audit['core_narrative_entities'], 23)
        self.assertEqual(self.audit['native_fields'], 675)
        self.assertEqual(self.audit['recorded_relationships'], 120)
        self.assertEqual(self.audit['physical_relationships'], 103)
        self.assertEqual(self.audit['application_relationships'], 17)
        self.assertEqual(self.audit['unknown_cardinalities'], [])
        self.assertEqual(len(self.audit['entity_coverage']), 76)
        for section in ('delivery-area','search-area','authority-area'):
            self.assertEqual(self.elements[section]['shape'], 'section')
            self.assertNotIn('parent',self.elements[section])

    def test_native_fields_and_relationship_pins_are_exact(self):
        native = [e for e in self.elements.values() if e.get('shape') == 'entity']
        self.assertEqual(len(native), len(self.pinned_entities))
        self.assertEqual({e['source']['target']: e['source'] for e in native}, self.pinned_entities)
        for e in self.elements.values():
            if e.get('shape')=='entity':
                self.assertTrue(e.get('summary'), e['id'])
                self.assertLessEqual(len(e['summary']), 120, e['id'])
                record=self.source(e['source'])
                self.assertEqual([(f['name'],f['type']) for f in e['fields']],
                                 [(f['name'],f['type']) for f in record['fields']])
                self.assertEqual(e['description'], record['explanation'])
                for actual, source in zip(e['fields'], record['fields']):
                    expected = set()
                    if 'primary' in source.get('keys', []): expected.add('pk')
                    if 'foreign' in source.get('keys', []): expected.add('fk')
                    self.assertEqual(set(actual.get('key', '').split(','))-{''}, expected)
        expected_relationships = {(pin['target'], r['id'])
                                  for pin in self.pinned_entities.values()
                                  for r in self.source(pin).get('relationships', [])}
        recorded = [item for item in self.evidence if item['recorded']]
        self.assertEqual(len(recorded), len(expected_relationships))
        self.assertEqual({(item['source']['target'], item['source']['relationship']) for item in recorded},
                         expected_relationships)
        for item in self.evidence:
            source=item['source']
            if 'relationship' not in source:
                self.assertIn(item['kind'], {'application_projection','transport_explanation',
                                            'authorization_explanation','same_entity_appearance'})
                continue
            r=next(r for r in self.source(source)['relationships'] if r['id']==source['relationship'])
            self.assertEqual(item['supporting_text'],r['explanation'])
            self.assertEqual(item['endpoint_pins'][1],r['destination'])
            self.assertEqual(item['endpoint_pins'][0],{k:source[k] for k in ('target','revision')})
            self.assertEqual(item['kind']=='physical_fk',r['explanation'].lower().startswith('physical'))
            self.assertEqual(item['code'], r['code'])
            self.assertEqual(item['cardinality'], r['cardinality'])
            connection = self.elements[item['id']]
            expected_markers = {'0..many': 'zero-or-many', '0..1': 'zero-or-one', '1': 'only-one'}
            self.assertEqual(connection['tail'], expected_markers[r['cardinality']['owner']])
            self.assertEqual(connection['head'], expected_markers[r['cardinality']['destination']])
            self.assertEqual(connection['head_size'], 100)
            for end, pin in zip(('from', 'to'), item['endpoint_pins']):
                self.assertEqual(self.elements[connection[end].split('/')[-1]]['source'], pin)
        # These two source relationships share endpoints but differ in direction
        # and meaning; neither may be collapsed into the other.
        parent = self.elements['cloud-projects--fk-policy-parent']
        current = self.elements['cloud-projects--fk-2']
        self.assertEqual((parent['tail'], parent['head']), ('only-one', 'zero-or-many'))
        self.assertEqual((current['tail'], current['head']), ('zero-or-one', 'only-one'))

    def test_full_source_metadata_is_retained_without_svg_dependency(self):
        self.assertEqual(len(self.entity_evidence), 76)
        for item in self.entity_evidence:
            self.assertEqual(item['record'], self.source(item['source']))
        # Running with every source-SVG read forbidden proves that coverage and
        # geometry do not depend on the old visual assets or imported bindings.
        read_bytes, read_text = Path.read_bytes, Path.read_text
        def guard_bytes(path, *args, **kwargs):
            self.assertNotEqual(path.suffix, '.svg', str(path))
            return read_bytes(path, *args, **kwargs)
        def guard_text(path, *args, **kwargs):
            self.assertNotEqual(path.suffix, '.svg', str(path))
            return read_text(path, *args, **kwargs)
        with tempfile.TemporaryDirectory() as output:
            with mock.patch.object(Path, 'read_bytes', guard_bytes), mock.patch.object(Path, 'read_text', guard_text), \
                    mock.patch.object(sys, 'argv', [str(SCRIPT), str(self.root), output]), mock.patch('builtins.print'):
                AUTHOR.main()

    def test_core_is_one_connected_model(self):
        native={e['id'] for e in self.elements.values() if e.get('shape')=='entity'}
        graph={key:set() for key in native}
        for item in self.evidence:
            a,b=(item[k].split('/')[-1] for k in ('from','to'))
            if a in native and b in native:
                graph[a].add(b)
                graph[b].add(a)
        disconnected = {'cloud-corpus-migration-scans', 'cloud-corpus-object-migrations',
                        'cloud-group-mapping-seed-state', 'local-agent-kv',
                        'local-episode-catalog-meta', 'local-local-search-meta'}
        # Start in the narrative instead of an arbitrary metadata-only table.
        reached = set()
        pending = ['local-pending-project-deliveries']
        while pending:
            key = pending.pop()
            if key not in reached:
                reached.add(key)
                pending.extend(graph[key]-reached)
        self.assertEqual(reached, native-disconnected)
        self.assertEqual(set(self.audit['disconnected_entities']), disconnected)
        self.assertTrue(all(not graph[k] for k in disconnected))

    def test_supporting_links_are_direct_native_details(self):
        core = set(AUTHOR.CORE_RELATIONSHIPS)
        for item in self.evidence:
            element = self.elements[item['id']]
            if item['recorded']:
                pair = (item['source']['target'].split(':')[-1], item['source']['relationship'])
                self.assertEqual(element.get('visibility') == 'detail', pair not in core)
            else:
                self.assertNotIn('tail', element)
                self.assertEqual(element['head'], 'arrow')
        self.assertEqual(sum(e.get('visibility') == 'detail' for e in self.elements.values()), 99)

    def test_output_cannot_be_written_into_source(self):
        result=subprocess.run([sys.executable,str(SCRIPT),str(self.root),str(self.root/'forbidden-output')],
                              capture_output=True,text=True)
        self.assertNotEqual(result.returncode,0)
        self.assertIn('output must be outside',result.stderr)
        self.assertFalse((self.root/'forbidden-output').exists())


if __name__ == '__main__':
    unittest.main()
