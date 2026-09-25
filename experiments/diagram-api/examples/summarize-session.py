#!/usr/bin/env python3
"""Summarize a logged.py session: totals and categories matching run.py."""
import json, pathlib, sys
log=pathlib.Path(sys.argv[1]);rows=[json.loads(l) for l in log.read_text().splitlines()]
def cat(r):
    c=r['argv'][1]
    return 'rendered_artifacts' if c=='render' else ('full_source_and_merge' if c in ['source','merge'] else 'compact_authoring_and_queries')
out={'commands':len(rows),'failed':[dict(command=r['argv'][1],exit=r['exit']) for r in rows if r['exit']],'categories':{},'token_counts':'not measured'}
for r in rows:
    v=out['categories'].setdefault(cat(r),dict(commands=0,request_bytes=0,response_bytes=0))
    v['commands']+=1;v['request_bytes']+=r['request_bytes'];v['response_bytes']+=r['response_bytes']
print(json.dumps(out,indent=2))
