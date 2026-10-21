#!/usr/bin/env python3
"""
Replayer "fresco": espera um novo request trn=605 (comprovante) do navegador e
reenvia VERBATIM (assinatura do proprio navegador) o mais rapido possivel.

Serve para distinguir:
  - replay OK   -> nosso request sintetico tem alguma falha
  - replay falha-> o servidor rejeita reuso (nonce) ou exige algo do cliente
"""
import json
import subprocess
import time

CAP = "/tmp/luggom-captura.json"
COOKIE = open("cookie.txt").read().strip()
UA = "Mozilla/5.0 (X11; Linux x86_64; rv:154.0) Gecko/20100101 Firefox/154.0"
BASE = "https://contaonline.viacredi.coop.br"

seen = set()
print("[replay] aguardando request fresco trn=605 ...", flush=True)

while True:
    try:
        d = json.load(open(CAP))
    except Exception:
        time.sleep(0.2); continue
    reqs = d.get("reqs", [])
    for q in reversed(reqs[-8:]):
        H = q.get("headers") or {}
        if not H.get("Access-Signature"):
            continue
        try:
            j = json.loads(q.get("body") or "{}")
        except Exception:
            continue
        if j.get("trn") != 605:
            continue
        aid = H.get("Access-Id")
        if aid in seen:
            continue
        seen.add(aid)
        age = time.time() * 1000 - int(H.get("Access-Time", "0"))
        if age > 8000:
            continue
        print(f"[replay] NOVO trn=605 seq={j.get('seqFlow')} idade={age:.0f}ms -> reenviando", flush=True)
        args = ["curl", "-sS", "-i", "--http2", "-X", "POST",
                BASE + "/servcorews/processJSON", "--data-binary", q["body"], "--compressed"]
        for k in ("Accept", "Content-Type", "Authorization", "Client-Id", "Access-Id",
                  "Access-Time", "Access-Origin", "Access-Uri", "Access-Signature",
                  "X-Request", "tracestate", "traceparent", "x-dtpc", "x-dtreferer"):
            if H.get(k):
                args += ["-H", f"{k}: {H[k]}"]
        args += ["-H", f"Origin: {BASE}", "-H", f"Referer: {BASE}/ib/dashboard",
                 "-H", f"User-Agent: {UA}", "-H", f"Cookie: {COOKIE}"]
        out = subprocess.run(args, capture_output=True, text=True, timeout=30)
        body = out.stdout.split("\r\n\r\n", 1)[-1].split("\n\n", 1)[-1]
        print("[replay] RESPOSTA:", body[:300], flush=True)
    time.sleep(0.2)
