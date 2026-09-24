#!/usr/bin/env python3
"""Create a disposable FeatureFlag demo using only public Saga authoring APIs.
Usage: python3 e2e/support/inventory-preview.py /path/to/change-saga [/tmp/new-dir]
The optional directory must not exist. No real Saga is read or changed.
"""
import json
import pathlib
import subprocess
import sys
import tempfile

binary = str(pathlib.Path(sys.argv[1]).resolve())
root = pathlib.Path(sys.argv[2]) if len(sys.argv) > 2 else pathlib.Path(tempfile.mkdtemp(prefix="csi-", dir="/tmp"))
if len(sys.argv) > 2:
    root.mkdir()
repo = root / "code"
repo.mkdir()
saga = root / "demo.saga"

def git(*args):
    return subprocess.check_output(["git", "-C", str(repo), *args], text=True, stderr=subprocess.DEVNULL).strip()

def cli(*args):
    result = subprocess.run([binary, *map(str, args)], text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(f"{args}: {result.stdout}\n{result.stderr}")
    return result.stdout

git("init", "-b", "main")
git("config", "user.name", "Disposable Demo")
git("config", "user.email", "demo@example.test")
git("remote", "add", "origin", "https://example.test/feature-flag-demo.git")
(repo / "flags.js").write_text('''// One shared flag system; feature callers do not own its definitions.
export class FlagStore {
  constructor(values) { this.values = values; }
  lookup(name) { return this.values.get(name) ?? false; }
}
export class FlagEvaluator {
  constructor(store) { this.store = store; }
  enabled(name) { return this.store.lookup(name) === true; }
}
export class FlagClient {
  constructor(evaluator) { this.evaluator = evaluator; }
  isEnabled(name) { return this.evaluator.enabled(name); }
}
export function checkout(client) {
  return client.isEnabled('express-checkout') ? 'express' : 'standard';
}
export function search(client) {
  return client.isEnabled('ranked-search') ? 'ranked' : 'chronological';
}
''')
git("add", ".")
git("commit", "-m", "Disposable FeatureFlag source")
commit = git("rev-parse", "HEAD")
cli("init", "--repo", repo, "--title", "Shared FeatureFlag documentation", saga)
urn = "urn:change-saga:demo:"
def ref(start, end, note):
    return {"commit": commit, "path": "flags.js", "start": start, "end": end, "note": note}
components = [
    ("client", "FlagClient", 10, 13, "Provides a stable Boolean flag API to feature callers. Checkout and search use this same client; neither duplicates evaluation logic."),
    ("evaluator", "FlagEvaluator", 6, 9, "Converts a stored value into a strict Boolean decision. Only true enables a feature, so unknown and non-Boolean values fail closed."),
    ("store", "FlagStore", 2, 5, "Owns flag values and the missing-key fallback. Unknown names return false; callers receive no storage details."),
]
for ident, name, start, end, explanation in components:
    source = root / f"{ident}.json"
    source.write_text(json.dumps({"name": name, "explanation": explanation, "code": [ref(start, end, explanation)]}))
    cli("component", "add", "--id", ident, "--from", source, "--repo", repo, "--json", saga)
pins = [{"target": urn + "component:" + c[0], "revision": urn + "component:" + c[0] + ":revision:r1"} for c in components]
interactions = [
    {"id": "evaluate", "from": pins[0]["target"], "to": pins[1]["target"], "description": "The client passes a named flag to the evaluator and returns its Boolean decision.", "code": [ref(12, 12, "Client delegates the exact named flag.")]},
    {"id": "lookup", "from": pins[1]["target"], "to": pins[2]["target"], "description": "The evaluator requests the stored value; the store falls back to false for an unknown name. Only true enables the feature.", "code": [ref(8, 8, "Strict Boolean evaluation."), ref(4, 4, "Missing-key fallback.")]},
]
system = {"name": "FeatureFlag", "explanation": "One shared decision path serves multiple features. A feature supplies a name through FlagClient; FlagEvaluator asks FlagStore for its value and enables only explicit true. Unknown names return false. Contextual slides explain their own feature's branch; linking this System never claims coverage of every flag consumer.", "code": [ref(12, 12, "The shared client entry point delegates evaluation.")], "components": pins, "interactions": interactions}
source = root / "system.json"
source.write_text(json.dumps(system))
cli("system", "add", "--id", "feature-flag", "--from", source, "--repo", repo, "--json", saga)
for feature, title, line, input_text, output_text in [
    ("checkout", "Checkout chooses an express path", 15, "express-checkout", "express / standard"),
    ("search", "Search selects a ranking strategy", 18, "ranked-search", "ranked / chronological"),
]:
    cli("feature", "add", "--id", feature, "--title", feature.title(), saga)
    cli("add-deck", "--feature", feature, "--id", feature + "-implementation", "--objective", title, saga, feature)
    visual = root / (feature + ".svg")
    visual.write_text(f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720">
<rect width="1280" height="720" fill="#f8f8f5"/>
<text x="72" y="88" font-family="Arial,sans-serif" font-size="20" fill="#666">{feature.upper()} · IMPLEMENTATION</text>
<text x="72" y="157" font-family="Arial,sans-serif" font-size="42" fill="#151d2b">{title}</text>
<text x="72" y="205" font-family="Arial,sans-serif" font-size="23" fill="#526071">A contextual decision, one shared FeatureFlag definition.</text>
<defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="9" markerHeight="9" orient="auto"><path d="M0 0L10 5L0 10Z" fill="#56677e"/></marker></defs>
<g id="input"><rect x="72" y="315" width="300" height="142" fill="#fff" stroke="#8b98a9"/><text x="98" y="365" font-family="Arial,sans-serif" font-size="20" fill="#526071">FLAG NAME</text><text x="98" y="411" font-family="Arial,sans-serif" font-size="25" fill="#151d2b">{input_text}</text></g>
<g id="shared"><rect x="472" y="295" width="330" height="182" fill="#e9effb" stroke="#37619c" stroke-width="2"/><text x="506" y="363" font-family="Arial,sans-serif" font-size="31" fill="#1b4278">FeatureFlag</text><text x="506" y="408" font-family="Arial,sans-serif" font-size="21" fill="#3f587a">Shared System · r1</text><text x="506" y="442" font-family="Arial,sans-serif" font-size="17" fill="#3f587a">Open its explanation and code</text></g>
<g id="result"><rect x="902" y="315" width="306" height="142" fill="#fff" stroke="#8b98a9"/><text x="924" y="365" font-family="Arial,sans-serif" font-size="20" fill="#526071">FEATURE BRANCH</text><text x="924" y="411" font-family="Arial,sans-serif" font-size="22" fill="#151d2b">{output_text}</text></g>
<g id="request"><path d="M372 386H466" stroke="#56677e" stroke-width="3" marker-end="url(#arrow)"/><text x="388" y="359" font-family="Arial,sans-serif" font-size="16" fill="#526071">name</text></g>
<g id="decision"><path d="M802 386H896" stroke="#56677e" stroke-width="3" marker-end="url(#arrow)"/><text x="815" y="359" font-family="Arial,sans-serif" font-size="16" fill="#526071">Boolean</text></g>
<text x="72" y="568" font-family="Arial,sans-serif" font-size="24" fill="#151d2b">Unknown flag → false → existing behavior</text>
<text x="72" y="612" font-family="Arial,sans-serif" font-size="20" fill="#526071">This slide owns only the {feature} branch at flags.js:{line}.</text>
</svg>''')
    cli("add-slide", "--deck", feature + "-implementation", "--id", feature + "-flag", "--title", title, "--intent", "explain", "--layout", "diagram", "--takeaway", "A shared flag decision selects this feature's behavior; unknown flags keep the existing path.", "--source", visual, saga, feature + "-flag")
    for ident, label, description in [
        ("input", "Flag name", f"{feature.title()} supplies {input_text}."),
        ("shared", "FeatureFlag", "The same shared System definition serves checkout and search."),
        ("result", "Feature branch", f"The Boolean selects {output_text}."),
        ("request", "Named request", "Pass the flag name through the shared client."),
        ("decision", "Boolean response", "The returned Boolean chooses the feature path."),
    ]:
        pin = ["--documentation", urn + "system:feature-flag", "--documentation-revision", urn + "system:feature-flag:revision:r1"] if ident == "shared" else []
        cli("add-item", "--slide", feature + "-flag", "--kind", "edge" if ident in ("request", "decision") else "node", "--id", ident, "--element-id", ident, "--label", label, "--description", description, *pin, saga)
        cli("cover", "--target", urn + "slide:" + feature + "-flag:item:" + ident, "--ref", f"{commit}:flags.js#L{line}", "--note", description, "--repo", repo, "--json", saga)
cli("validate", "--json", saga)
(root / "preview.json").write_text(json.dumps({"root": str(root), "repo": str(repo), "saga": str(saga), "system": urn + "system:feature-flag", "commit": commit}, indent=2))
print((root / "preview.json").read_text())
