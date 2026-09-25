#!/usr/bin/env python3
"""Run one diagram-spike command and append its measured traffic to a session log.

Usage: logged.py LOG.jsonl -- ARGS...   (stdin, stdout, stderr and exit pass through)
Counts match run.py: argv bytes (space-separated), stdin, stdout and stderr.
"""
import json, pathlib, subprocess, sys
ROOT=pathlib.Path(__file__).resolve().parents[1]
log=pathlib.Path(sys.argv[1]);assert sys.argv[2]=='--'
command=[str(ROOT/'bin'/'diagram-spike')]+sys.argv[3:]
payload=b'' if sys.stdin.isatty() else sys.stdin.buffer.read()
p=subprocess.run(command,input=payload,capture_output=True)
with log.open('a') as f:
    f.write(json.dumps(dict(argv=command,request_bytes=len(' '.join(command).encode())+len(payload),response_bytes=len(p.stdout)+len(p.stderr),exit=p.returncode))+'\n')
sys.stdout.buffer.write(p.stdout);sys.stderr.buffer.write(p.stderr);sys.exit(p.returncode)
