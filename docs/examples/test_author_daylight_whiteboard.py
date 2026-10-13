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
        cls.elements = {e['id']: e for e in cls.board['elements']}

    def source(self, pin):
        key, revision = pin['target'].split(':')[-1], pin['revision'].split(':')[-1]
        return json.loads((self.root/'___inventory'/'data-entities'/(key+'.data-entity')/'revisions'/(revision+'.json')).read_text())

    def test_source_tree_is_unchanged_and_every_pin_remains(self):
        self.assertEqual(self.before, {p: AUTHOR.file_digest(p) for p in self.root.rglob('*') if p.is_file()})
        frames = [e for e in self.elements.values() if e['kind']=='frame']
        self.assertEqual({(e['id'],e['source']['revision'].split(':')[-1]) for e in frames},
                         {(key,rev) for key,rev,_ in AUTHOR.PINS})
        self.assertEqual(self.audit['covered_entities'], 76)
        self.assertEqual(self.audit['primary_native_entities'], 23)
        self.assertEqual(len(self.audit['entity_coverage']), 76)
        for section in ('delivery-area','search-area','authority-area'):
            self.assertEqual(self.elements[section]['shape'], 'section')
            self.assertNotIn('parent',self.elements[section])

    def test_native_fields_and_relationship_pins_are_exact(self):
        for e in self.elements.values():
            if e.get('shape')=='entity':
                self.assertTrue(e.get('summary'), e['id'])
                self.assertLessEqual(len(e['summary']), 120, e['id'])
                record=self.source(e['source'])
                self.assertEqual([(f['name'],f['type']) for f in e['fields']],
                                 [(f['name'],f['type']) for f in record['fields']])
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

    def test_core_is_one_connected_model(self):
        native={e['id'] for e in self.elements.values() if e.get('shape')=='entity'}
        graph={key:set() for key in native}
        for item in self.evidence:
            a,b=(item[k].split('/')[-1] for k in ('from','to'))
            if a in native and b in native:
                graph[a].add(b)
                graph[b].add(a)
        reached=set()
        pending=[next(iter(native))]
        while pending:
            key=pending.pop()
            if key not in reached:
                reached.add(key)
                pending.extend(graph[key]-reached)
        self.assertEqual(reached,native)

    def test_output_cannot_be_written_into_source(self):
        result=subprocess.run([sys.executable,str(SCRIPT),str(self.root),str(self.root/'forbidden-output')],
                              capture_output=True,text=True)
        self.assertNotEqual(result.returncode,0)
        self.assertIn('output must be outside',result.stderr)
        self.assertFalse((self.root/'forbidden-output').exists())


if __name__ == '__main__':
    unittest.main()
