#!/usr/bin/env python3
"""Gera um request assinado e envia via curl (permite --http2 e headers browser-like)."""
import json
import subprocess
import sys

sys.path.insert(0, ".")
from synth_request import build  # noqa: E402

cap = sys.argv[1] if len(sys.argv) > 1 else "cap4.json"
trn = int(sys.argv[2]) if len(sys.argv) > 2 else 549
seqarg = sys.argv[3] if len(sys.argv) > 3 else "none"
seq = None if seqarg.lower() in ("none", "null", "-1") else int(seqarg)
data = json.loads(sys.argv[4]) if len(sys.argv) > 4 else {}

extra = [
    ["Accept-Language", "pt-BR,pt;q=0.8,en-US;q=0.5,en;q=0.3"],
    ["Accept-Encoding", "gzip, deflate, br, zstd"],
    ["Sec-Fetch-Dest", "empty"],
    ["Sec-Fetch-Mode", "cors"],
    ["Sec-Fetch-Site", "same-origin"],
    ["Priority", "u=0"],
]

body_s, headers, canon, spki, _op = build(cap, "x_response.txt", "cookie.txt", trn, seq, data)
args = ["curl", "-sS", "-i", "--http2", "-X", "POST",
        "https://contaonline.viacredi.coop.br/servcorews/processJSON",
        "--data-binary", body_s, "--compressed"]
for k, v in headers + extra:
    args += ["-H", f"{k}: {v}"]

print(f"trn={trn} seqFlow={seq} data={data}  (enviando via curl --http2)")
out = subprocess.run(args, capture_output=True, text=True, timeout=40)
first = out.stdout.find("HTTP/")
print(out.stdout[first:first+180])
print("--- BODY (tail) ---")
print(out.stdout[-400:])
if out.stderr:
    print("STDERR:", out.stderr[:500])
