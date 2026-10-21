#!/usr/bin/env python3
"""
Reproduz EXATAMENTE a assinatura do app Viacredi (Ailos/servcore), conforme o
proprio bundle JS (main.*.js):

  canonical = accessId:accessTime:origin:uri:body:Bearer <token>
      body  = JSON.stringify(body).replace(/\s/g,"") + NFD sem acentos
  Access-Signature = ECDSA P-256/SHA-256(canonical) em DER -> base64
  X-Request = RSA-OAEP(sha256, mgf1=sha1) sobre JSON.stringify(spki_base64)
              cifrado com a chave x_response -> base64

Uso: python3 synth_request.py <trn> <seqflow> '<json-data>' [outfile]
"""
import base64
import hashlib
import http.cookiejar
import json
import os
import sys
import unicodedata
import urllib.request
import urllib.error
import uuid

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, padding

_JAR = None


def opener_for(cookie_str):
    """Cookie jar semeado com a cookie string; persiste cookies novos (F5/Volterra)."""
    global _JAR
    _JAR = http.cookiejar.CookieJar()
    for pair in cookie_str.split("; "):
        if "=" not in pair:
            continue
        k, v = pair.split("=", 1)
        c = http.cookiejar.Cookie(
            version=0, name=k, value=v, port=None, port_specified=False,
            domain="contaonline.viacredi.coop.br", domain_specified=True,
            domain_initial_dot=False, path="/", path_specified=True,
            secure=True, expires=None, discard=False, comment=None,
            comment_url=None, rest={}, rfc2109=False)
        _JAR.set_cookie(c)
    return urllib.request.build_opener(urllib.request.HTTPCookieProcessor(_JAR))

BASE = "https://contaonline.viacredi.coop.br"
ORIGIN = BASE
URI = BASE + "/ib/auth"
UA = "Mozilla/5.0 (X11; Linux x86_64; rv:154.0) Gecko/20100101 Firefox/154.0"


def load_material(tokcap, xrespfile, cookiefile):
    d = json.load(open(tokcap))
    req = None
    for q in reversed(d["reqs"]):
        h = q.get("headers") or {}
        if h.get("Authorization") and h.get("Client-Id"):
            req = q
            break
    if req is None:
        raise SystemExit("nao achei request com Authorization+Client-Id na captura")
    token = req["headers"]["Authorization"].split(" ", 1)[1]
    client_id = req["headers"]["Client-Id"]
    xresp = open(xrespfile).read().strip()
    cookie = open(cookiefile).read().strip()
    return token, client_id, xresp, cookie


def get_system_ts(cookie):
    op = opener_for(cookie)
    req = urllib.request.Request(BASE + "/servcorews/getSystemTS",
                                 headers={"Accept": "application/json, text/plain, */*",
                                          "User-Agent": UA})
    with op.open(req, timeout=20) as r:
        raw = r.read().decode("utf-8", "replace").strip()
    # pode vir numero puro ou XML
    if raw.startswith("<"):
        import re
        raw = re.search(r">([0-9]+)<", raw).group(1)
    return raw, op


def norm_body(body_str):
    compact = json.dumps(json.loads(body_str), separators=(",", ":"), ensure_ascii=False)
    return unicodedata.normalize("NFD", compact)


def build(tokcap, xrespfile, cookiefile, trn, seqflow, data):
    token, client_id, xresp, cookie = load_material(tokcap, xrespfile, cookiefile)
    atime, op = get_system_ts(cookie)
    access_id = str(uuid.uuid4())

    body = {"fi": 7, "chn": "3", "trn": trn}
    if seqflow is not None:
        body["seqFlow"] = seqflow
    body["data"] = data
    body_s = norm_body(json.dumps(body))

    canon = f"{access_id}:{atime}:{ORIGIN}:{URI}:{body_s}:Bearer {token}"
    priv = ec.generate_private_key(ec.SECP256R1())
    der = priv.sign(canon.encode("utf-8"), ec.ECDSA(hashes.SHA256()))
    access_sig = base64.b64encode(der).decode()
    spki = base64.b64encode(priv.public_key().public_bytes(
        serialization.Encoding.DER, serialization.PublicFormat.SubjectPublicKeyInfo)).decode()

    rsa = serialization.load_der_public_key(base64.b64decode(xresp))
    plaintext = json.dumps(spki).encode("utf-8")  # JSON.stringify -> com aspas
    ct = rsa.encrypt(plaintext, padding.OAEP(
        mgf=padding.MGF1(algorithm=hashes.SHA1()),
        algorithm=hashes.SHA256(), label=None))
    x_request = base64.b64encode(ct).decode()

    headers = [
        ["Accept", "application/json, text/plain, */*"],
        ["Content-Type", "application/json"],
        ["Authorization", f"Bearer {token}"],
        ["Client-Id", client_id],
        ["Access-Id", access_id],
        ["Access-Time", atime],
        ["Access-Origin", ORIGIN],
        ["Access-Uri", URI],
        ["Access-Signature", access_sig],
        ["X-Request", x_request],
        ["Origin", ORIGIN],
        ["Referer", BASE + "/ib/dashboard"],
        ["User-Agent", UA],
    ]
    return body_s, headers, canon, spki, op


def send(body_s, headers, op):
    h = {k: v for k, v in headers}
    req = urllib.request.Request(BASE + "/servcorews/processJSON",
                                 data=body_s.encode("utf-8"), headers=h, method="POST")
    try:
        with op.open(req, timeout=30) as r:
            print("--- response headers ---")
            for k, v in r.headers.items():
                print(f"   {k}: {v}")
            return r.status, r.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        print("--- response headers ---")
        for k, v in e.headers.items():
            print(f"   {k}: {v}")
        return e.code, e.read().decode("utf-8", "replace")
    except Exception as e:
        return None, f"{type(e).__name__}: {e}"


if __name__ == "__main__":
    cap = sys.argv[1] if len(sys.argv) > 1 else "/tmp/luggom-captura.json"
    xr = sys.argv[2] if len(sys.argv) > 2 else "x_response.txt"
    ck = sys.argv[3] if len(sys.argv) > 3 else "cookie.txt"
    trn = int(sys.argv[4]) if len(sys.argv) > 4 else 605
    seqarg = sys.argv[5] if len(sys.argv) > 5 else "11"
    seq = None if seqarg.lower() in ("none", "null", "-1") else int(seqarg)
    data = json.loads(sys.argv[6]) if len(sys.argv) > 6 else {"ID_TRANSACAO": "867596174"}

    body_s, headers, canon, spki, op = build(cap, xr, ck, trn, seq, data)
    print(f"trn={trn} seqFlow={seq} data={data}")
    print(f"canonical({len(canon)}): {canon[:150]}...")
    print(f"spki({len(spki)}): {spki[:48]}...")
    st, resp = send(body_s, headers, op)
    print(f"\nHTTP {st}")
    print(resp[:900])
