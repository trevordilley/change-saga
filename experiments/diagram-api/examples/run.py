#!/usr/bin/env python3
"""Reproducible CLI experiment. Outputs stay in a fresh, caller-chosen directory."""
import copy, json, pathlib, subprocess, sys
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=pathlib.Path(sys.argv[1] if len(sys.argv)>1 else ROOT/'runs'/'demo').resolve()
OUT.mkdir(parents=True,exist_ok=False)
BIN=ROOT/'bin'/'diagram-spike'
STORE=OUT/'diagram'
traffic=[]
def run(args, data=None, artifact=None, okay=True, measured=True):
    payload=b'' if data is None else (json.dumps(data,separators=(',',':'))+'\n').encode()
    command=[str(BIN)]+args
    p=subprocess.run(command,input=payload,capture_output=True)
    # Count actual argv bytes separated by spaces, stdin, stdout and stderr.
    # Paths are retained so the report states exactly which run was measured.
    if measured:traffic.append(dict(argv=command,request_bytes=len(' '.join(command).encode())+len(payload),response_bytes=len(p.stdout)+len(p.stderr),exit=p.returncode))
    if artifact:(OUT/artifact).write_bytes(p.stdout)
    if p.returncode and okay:raise RuntimeError(p.stderr.decode())
    if not okay and p.returncode==0:raise RuntimeError('Expected failure: '+' '.join(command))
    return json.loads(p.stdout) if p.stdout and artifact is None else p.stdout
run(['schema'],artifact='schema.json')
run(['assets','search','database'],artifact='asset-search.json')
r=run(['init','--store',str(STORE),'--id','atomic','--title','Atomic diagram publication','--request','init'])
scene=json.loads((ROOT/'examples/scene.json').read_text())
req=dict(version=1,request_id='compose',expected_snapshot=r['snapshot'],operations=[dict(op='add',element=e) for e in scene])
(OUT/'create-request.json').write_text(json.dumps(req,indent=2)+'\n')
r=run(['apply','--store',str(STORE)],req)
run(['describe','--store',str(STORE)],artifact='describe-before.txt')
run(['render','--store',str(STORE)],artifact='before.svg')
# Two actual source branches from a common baseline (no DevSwarm workspaces).
base=run(['source','--store',str(STORE)])
for name,d in [('base',base),('ours',copy.deepcopy(base)),('theirs',copy.deepcopy(base))]:
    if name=='ours':d['elements']['author']['label']='Author edits'
    if name=='theirs':d['elements']['published']['label']='New snapshot'
    (OUT/(name+'.json')).write_text(json.dumps(d,indent=2)+'\n')
run(['merge','--base',str(OUT/'base.json'),'--ours',str(OUT/'ours.json'),'--theirs',str(OUT/'theirs.json')],artifact='merged.json')
same=copy.deepcopy(base);same['elements']['author']['x']+=10;(OUT/'same.json').write_text(json.dumps(same,indent=2)+'\n')
run(['merge','--base',str(OUT/'base.json'),'--ours',str(OUT/'ours.json'),'--theirs',str(OUT/'same.json')],artifact='merge-conflict.json',okay=False)
# An explicitly sized label failure and a deliberate correction.
run(['update','--store',str(STORE),'--expected',r['snapshot'],'--request','overflow','--id','validator','--set',json.dumps({'label':'This deliberately long description does not fit in its label slot'})],okay=False)
r=run(['move','--store',str(STORE),'--expected',r['snapshot'],'--request','move','--id','validator','--dy','30'])
run(['render','--store',str(STORE)],artifact='moved-unrepaired.svg')
run(['get','--store',str(STORE),'--id','publish'],artifact='targeted-edge.json')
# The author fixes each affected path explicitly; no routing engine participates.
ops=[dict(op='update',id='validator',set={'label':'Validate batch','style':'normal'}),
     dict(op='update',id='batch',set={'points':[{'x':345,'y':280},{'x':410,'y':280},{'x':410,'y':310},{'x':445,'y':310}]}),
     dict(op='update',id='publish',set={'points':[{'x':775,'y':310},{'x':845,'y':310},{'x':845,'y':280},{'x':920,'y':280}]}),
     dict(op='update',id='failure',set={'points':[{'x':610,'y':385},{'x':610,'y':495},{'x':375,'y':495}]}),
     dict(op='add',element=dict(id='temporary',kind='edge',style='secondary',**{'from':'published','to':'validator'},points=[{'x':920,'y':350},{'x':775,'y':350}],head='arrow'))]
req=dict(version=1,request_id='revise',expected_snapshot=r['snapshot'],operations=ops)
r=run(['apply','--store',str(STORE)],req)
run(['apply','--store',str(STORE)],req) # identical retry
r=run(['remove','--store',str(STORE),'--expected',r['snapshot'],'--request','remove-edge','--id','temporary'])
run(['describe','--store',str(STORE)],artifact='describe-after.txt')
# Same projection for tooling; kept out of traffic so command indices stay comparable.
run(['describe','--store',str(STORE),'--format','json'],artifact='describe-after.json',measured=False)
run(['render','--store',str(STORE)],artifact='after.svg')
run(['check','--store',str(STORE)],artifact='integrity.json')
# Explicit corruption/recovery test keeps modified visual bytes.
record=json.loads((STORE/'current.json').read_text());visual=STORE/'visuals'/record['svg_hash'].split(':')[1]
visual.write_bytes(b'external SVG edit retained by rebuild\n')
run(['check','--store',str(STORE)],artifact='divergence.json',okay=False)
run(['rebuild','--store',str(STORE)],artifact='rebuild.json')
run(['render','--store',str(STORE)],artifact='rebuilt.svg')
assert (OUT/'after.svg').read_bytes()==(OUT/'rebuilt.svg').read_bytes()
report={'commands':len(traffic),'request_bytes':sum(t['request_bytes'] for t in traffic),'response_bytes':sum(t['response_bytes'] for t in traffic),'token_counts':'not measured','traffic':traffic}
report['describe_formats']={'text_bytes':(OUT/'describe-after.txt').stat().st_size,'json_bytes':(OUT/'describe-after.json').stat().st_size}
# SVG/font transfer and merge/source export are separate categories, not hidden costs.
report['categories']={}
for t in traffic:
    cat='rendered_artifacts' if t['argv'][1]=='render' else ('full_source_and_merge' if t['argv'][1] in ['source','merge'] else 'compact_authoring_and_queries')
    v=report['categories'].setdefault(cat,dict(commands=0,request_bytes=0,response_bytes=0));v['commands']+=1;v['request_bytes']+=t['request_bytes'];v['response_bytes']+=t['response_bytes']
(OUT/'traffic.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({'output':str(OUT),'commands':report['commands'],'request_bytes':report['request_bytes'],'response_bytes':report['response_bytes']}))
