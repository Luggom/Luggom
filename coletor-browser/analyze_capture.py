#!/usr/bin/env python3
"""
Analisador da captura do Luggom Coletor.

Objetivo: provar/refutar, sem tocar no banco, o esquema de assinatura da API
(Ailos/servcore) usando o que o PROPRIO NAVEGADOR assinou.

Para cada assinatura capturada (crypto.subtle.sign observado):
  1. verifica ECDSA P-256 / SHA-256 contra a chave efemera (SPKI) pareada;
  2. reconstroi a string canonica a partir dos headers/corpo do request e
     compara com o que o navegador assinou (mostra o diff exato).

Uso: python3 analyze_capture.py [caminho.json]
"""
import base64
import json
import sys
import unicodedata
from urllib.parse import urlparse

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, utils as asym_utils

DEFAULT = "/tmp/luggom-captura.json"


def b64d(s):
    if not s:
        return None
    try:
        return base64.b64decode(s)
    except Exception:
        return None


def try_verify(pub, sig, data):
    """Devolve o formato que verificou ('DER'/'P1363') ou None."""
    try:
        pub.verify(sig, data, ec.ECDSA(hashes.SHA256()))
        return "DER"
    except Exception:
        pass
    if len(sig) == 64:
        r = int.from_bytes(sig[:32], "big")
        s = int.from_bytes(sig[32:], "big")
        try:
            pub.verify(asym_utils.encode_dss_signature(r, s), data, ec.ECDSA(hashes.SHA256()))
            return "P1363"
        except Exception:
            pass
    return None


def hget(headers, name):
    n = name.lower()
    for k, v in (headers or {}).items():
        if k.lower() == n:
            return v
    return None


def norm_body(body):
    """JSON.stringify sem espacos + NFD (sem acentos)."""
    if body is None:
        return None
    try:
        obj = json.loads(body)
        compact = json.dumps(obj, separators=(",", ":"), ensure_ascii=False)
    except Exception:
        compact = body
    return unicodedata.normalize("NFD", compact)


def canon_from_req(req):
    h = req.get("headers") or {}
    aid = hget(h, "Access-Id")
    atime = hget(h, "Access-Time")
    aorigin = hget(h, "Access-Origin")
    auri = hget(h, "Access-Uri")
    auth = hget(h, "Authorization")
    corpo = norm_body(req.get("body"))
    if not all([aid, atime, aorigin, auri, auth, corpo]):
        return None, {"Access-Id": aid, "Access-Time": atime, "Access-Origin": aorigin,
                      "Access-Uri": auri, "Authorization": bool(auth), "corpo": bool(corpo)}
    return f"{aid}:{atime}:{aorigin}:{auri}:{corpo}:{auth}", None


def browser_corpo(canon, req):
    """Extrai o corpo que o navegador assinou, usando prefixo/sufixo dos headers."""
    h = req.get("headers") or {}
    prefix = f"{hget(h,'Access-Id')}:{hget(h,'Access-Time')}:{hget(h,'Access-Origin')}:{hget(h,'Access-Uri')}:"
    suffix = f":{hget(h,'Authorization')}"
    if canon.startswith(prefix) and canon.endswith(suffix):
        return canon[len(prefix):-len(suffix)]
    return None


def diff_str(a, b):
    """Primeira posicao onde divergem + janela."""
    if a == b:
        return "IGUAL"
    n = min(len(a), len(b))
    i = 0
    while i < n and a[i] == b[i]:
        i += 1
    lo = max(0, i - 40)
    return (f"divergem em offset {i} (len a={len(a)} b={len(b)})\n"
            f"   capturado : ...{a[lo:i+60]!r}\n"
            f"   reconstruido: ...{b[lo:i+60]!r}")


def main():
    path = sys.argv[1] if len(sys.argv) > 1 else DEFAULT
    try:
        with open(path, "rb") as f:
            cap = json.load(f)
    except FileNotFoundError:
        print(f"[!] captura nao encontrada: {path}")
        print("    Carregue o coletor no Firefox, va ao comprovante e deixe gravar.")
        return 2
    except Exception as e:
        print(f"[!] nao consegui ler {path}: {e}")
        return 2

    signs = cap.get("signs", [])
    reqs = cap.get("reqs", [])
    resps = cap.get("resps", [])
    print(f"captura: {path}")
    print(f"  signs={len(signs)}  reqs={len(reqs)}  resps={len(resps)}")
    print("=" * 70)

    # ---- 1) ECDSA ----
    ok = fail = no_key = 0
    keycache = {}
    print("\n[1] VERIFICACAO ECDSA P-256 / SHA-256 (assinatura do navegador)")
    for i, s in enumerate(signs):
        canon = s.get("canonical")
        sig = b64d(s.get("signature"))
        spki = s.get("spki")
        if not spki:
            no_key += 1
            continue
        pub = keycache.get(spki)
        if pub is None:
            try:
                pub = serialization.load_der_public_key(b64d(spki))
            except Exception as e:
                print(f"  #{i}: SPKI invalido ({e})")
                fail += 1
                continue
            keycache[spki] = pub
        ver = try_verify(pub, sig, canon.encode("utf-8")) if (sig and canon) else None
        if ver:
            ok += 1
        else:
            fail += 1
            print(f"  #{i}: NAO VERIFICOU  canon={canon[:80]!r}")
    print(f"  verificadas={ok}  falharam={fail}  sem_spki={no_key}  (chaves unicas={len(keycache)})")
    if ok and not fail:
        print("  => ESQUEMA CONFIRMADO: e exatamente ECDSA P-256/SHA-256 sobre a canonica capturada.")

    # ---- 2) canonica reconstruida vs capturada ----
    print("\n[2] CANONICA reconstruida (headers/corpo) vs assinada pelo navegador")
    matches = mismatch = incomplete = 0
    for i, r in enumerate(reqs):
        auth = hget(r.get("headers") or {}, "Authorization")
        aid = hget(r.get("headers") or {}, "Access-Id")
        if not aid:
            continue
        rec, missing = canon_from_req(r)
        if rec is None:
            incomplete += 1
            print(f"  req#{i} {r.get('method')} {urlparse(r.get('url','')).path}")
            print(f"    faltam headers: {missing}")
            # tentativa parcial: sem o Authorization, compara o prefixo
            m = missing
            if m.get("Authorization") is False and m.get("corpo") and m.get("Access-Id"):
                partial = f"{m['Access-Id']}:{m['Access-Time']}:{m['Access-Origin']}:{m['Access-Uri']}:{norm_body(r.get('body')) or ''}:"
                cand = [s.get("canonical") for s in signs if s.get("canonical") and s["canonical"].startswith(partial)]
                print(f"    prefixo (sem Authorization) {'BATE' if cand else 'NAO BATE'} "
                      f"{'(Bearer presente na captura)' if cand else ''}")
            continue
        hit = None
        for s in signs:
            if s.get("canonical") == rec:
                hit = "exato"
                break
        if hit is None:
            # procura a canonica capturada com mesmo Access-Id p/ mostrar diff
            cand = [s.get("canonical") for s in signs if s.get("canonical") and aid in s["canonical"]]
            if cand:
                print(f"  req#{i} {r.get('method')} {urlparse(r.get('url','')).path}")
                print(f"    {diff_str(cand[0], rec)}")
                # diagnostico do corpo (quando a divergencia nao e so o corpo)
                body_browser = browser_corpo(cand[0], r)
                body_ours = norm_body(r.get("body"))
                if body_browser is not None and body_browser != (body_ours or ""):
                    print(f"    corpo navegador : {body_browser[:200]!r}")
                    print(f"    corpo nosso     : {(body_ours or '')[:200]!r}")
                mismatch += 1
            else:
                incomplete += 1
                print(f"  req#{i}: sem canonica capturada com o mesmo Access-Id")
            continue
        matches += 1
    print(f"  exatos={matches}  divergentes={mismatch}  incompletos={incomplete}")

    # ---- 3) respostas com erro ----
    print("\n[3] RESPOSTAS (procurando o muro / RESPONSE_CODE)")
    for i, rp in enumerate(resps):
        body = rp.get("body") or ""
        if '"ERROR":true' in body or "RESPONSE_CODE" in body or rp.get("status", 200) >= 400:
            print(f"  resp#{i} status={rp.get('status')} {urlparse(rp.get('url','')).path}")
            print(f"    {body[:260]!r}")

    # ---- 4) operacoes de cripto (X-Request: RSA-OAEP etc.) ----
    cryptos = cap.get("cryptos", [])
    print(f"\n[4] OPERACOES crypto.subtle ({len(cryptos)})")
    for i, c in enumerate(cryptos):
        alg = c.get("alg") or {}
        pt = c.get("plaintext") or ""
        ct = c.get("ciphertext") or ""
        print(f"  #{i} {c.get('op')} name={alg.get('name')} hash={alg.get('hash')} label={alg.get('label')} "
              f"| pt={len(pt)}b64 ct={len(ct)}b64")
        if c.get("op") == "encrypt" and alg.get("name") == "RSA-OAEP":
            try:
                print(f"      plaintext: {base64.b64decode(pt).decode('utf-8', 'replace')[:220]!r}")
            except Exception:
                pass

    return 0


if __name__ == "__main__":
    sys.exit(main())
