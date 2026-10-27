#!/usr/bin/env python3
"""Scrive THIRD_PARTY_LICENSES.txt con le licenze di tutto ciò che finisce
dentro gli eseguibili: i moduli Go (letti dagli eseguibili già compilati in
build/bin) e mermaid con le librerie che contiene.

    python3 scripts/licenses.py
"""
import glob
import os
import re
import subprocess
import sys

root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
os.chdir(root)
modcache = subprocess.run(["go", "env", "GOMODCACHE"], capture_output=True, text=True, check=True).stdout.strip()

binaries = [b for b in ["build/bin/mdview.exe", "build/bin/mdview-linux-amd64", "build/bin/mdview-linux-arm64"] if os.path.exists(b)]
if not binaries:
    sys.exit("prima compila gli eseguibili in build/bin")

deps = set()
for b in binaries:
    out = subprocess.run(["go", "version", "-m", b], capture_output=True, text=True, check=True).stdout
    for line in out.splitlines():
        f = line.split()
        if len(f) >= 3 and f[0] == "dep":
            deps.add((f[1], f[2]))


def escape(path):
    return re.sub(r"[A-Z]", lambda m: "!" + m.group(0).lower(), path)


def license_text(path, version):
    d = f"{modcache}/{escape(path)}@{version}"
    files = sorted(f for f in glob.glob(d + "/*") if re.match(r"(?i)(licen[cs]e|copying)", os.path.basename(f)))
    if not files:
        sys.exit(f"licenza non trovata per {path}")
    return open(files[0], encoding="utf-8", errors="replace").read().strip()


sep = "\n\n" + "=" * 78 + "\n"
parts = [
    "MdView contiene software di terze parti. Qui sotto le loro licenze.\n"
    "MdView stesso è distribuito con licenza MIT (vedi LICENSE)."
]
for path, version in sorted(deps):
    parts.append(f"{path} {version}\n\n{license_text(path, version)}")
parts.append(
    "mermaid 12.2.0 (frontend: diagrammi)\n"
    "Il pacchetto contiene anche lodash, KaTeX, cytoscape, d3, marked, roughjs e\n"
    "dayjs (licenze MIT o ISC, i cui avvisi restano dentro il file di mermaid) e\n"
    "DOMPurify, che usiamo con licenza Apache 2.0 (testo più sotto).\n\n"
    + open("third_party/licenses/mermaid.txt", encoding="utf-8").read().strip()
)
parts.append("DOMPurify 3.4.12 (dentro mermaid)\n\n" + open("third_party/licenses/dompurify.txt", encoding="utf-8").read().strip())

with open("THIRD_PARTY_LICENSES.txt", "w", encoding="utf-8") as f:
    f.write(sep.join(parts) + "\n")
print(f"THIRD_PARTY_LICENSES.txt: {len(deps)} moduli Go + mermaid")
