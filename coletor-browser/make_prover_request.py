#!/usr/bin/env python3
"""
Gera o LUGGOM_REQUEST_FILE do prover TLSNotary: um request processJSON fresco,
assinado por nos (ECDSA P-256) e com o X-Request cifrado com a chave CORRETA.

CORRECCAO 2026-09-11 (bug do 401 de 2026-09-10):
  O X-Request NAO usa a `PUBLIC_RSA` do JWT. O browser usa
  `window.encryptionLib.publicKey`, que o fluxo de login preenche com a
  `x_response` devolvida pelo servidor (`encryptionService.publicKey =
  ...WS_RESULT.x_response`); so cai para a do JWT se nunca tiver sido definida
  (ver `createCipher()` no bundle). Prova: num X-Request capturado,
  c >= n(PUBLIC_RSA do JWT) -> e IMPOSSIVEL ter sido cifrado com essa chave.

  Ordem de preferencia da chave (a primeira que existir):
    1. `encKeyAtRequest.encKey` do proprio request capturado (o browser diz-nos
       qual a chave em uso no instante em que calculou o X-Request);
    2. a ultima `x_response` vista nas respostas;
    3. `PUBLIC_RSA` do JWT (fallback historico).

  Os cookies vem do `document.cookie` capturado (nenhum cookie da Viacredi e
  httpOnly) — dispensa DevTools; `cookie.txt` fica so como fallback.

Uso: python3 make_prover_request.py <captura.json> <trn> <seqflow|none> '<json-data>' <saida.json>
"""
import base64
import json
import os
import sys
import unicodedata
import uuid

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, padding

BASE = "https://contaonline.viacredi.coop.br"
PATH = "/servcorews/processJSON"
UA = "Mozilla/5.0 (X11; Linux x86_64; rv:154.0) Gecko/20100101 Firefox/154.0"
FALLBACK_URI = BASE + "/ib/auth"


def hget(headers, name):
    n = name.lower()
    for k, v in (headers or {}).items():
        if k.lower() == n:
            return v
    return None


def jwt_public_rsa(token):
    sub = json.loads(base64.urlsafe_b64decode(
        token.split(".")[1] + "=" * (-len(token.split(".")[1]) % 4)))["sub"]
    return json.loads(base64.urlsafe_b64decode(
        sub + "=" * (-len(sub) % 4)))["values"].get("PUBLIC_RSA")


def pick_key(cap):
    """Devolve (chave_b64, origem). Ordem deliberada:

    O `createCipher()` do app usa a `PUBLIC_RSA` do JWT sempre que existe token
    (e a `x_response` so quando ainda nao ha token, i.e. pre-login). Como o
    login recarrega a pagina, o service nasce de novo e os requests que nos
    interessam (comprovantes, pos-login) usam a chave do JWT. Confirmado ao
    vivo: com a chave do JWT o banco devolve 200 + o comprovante; com a
    `x_response` devolve RESPONSE_CODE=2 ("Algo inesperado aconteceu").
    """
    for q in reversed(cap.get("reqs", [])):
        ek = (q.get("encKeyAtRequest") or {}).get("encKey")
        if ek:
            return ek, "encKeyAtRequest (chave em uso no browser)"
    for q in reversed(cap.get("reqs", [])):
        h = q.get("headers") or {}
        if h.get("Authorization"):
            v = jwt_public_rsa(h["Authorization"].split(" ", 1)[1])
            if v:
                return v, "PUBLIC_RSA do JWT (pos-login)"
    for rp in reversed(cap.get("resps", [])):
        b = rp.get("body") or ""
        if "x_response" not in b:
            continue
        try:
            v = json.loads(b)["data"]["OUTPUT_DATA"]["WS_RESULT"]["x_response"]
            if v:
                return v, "x_response (pre-login)"
        except Exception:
            continue
    raise SystemExit("nao encontrei chave RSA na captura")


# Cookies sem papel na sessao (analytics/marketing/consentimento). O prover
# commita o pedido inteiro em MPC e o record layer tem um teto de 4096+32 bytes,
# que o pedido real da Viacredi estoura com eles; corta-los libera ~560 bytes.
COOKIES_IGNORADOS = ("_gcl_au", "_pk_ref", "_pk_id", "_pk_ses", "OptanonConsent", "rxVisitor")


def prune_cookie(ck):
    parts = [p.strip() for p in (ck or "").split(";") if p.strip()]
    return "; ".join(p for p in parts if not p.split("=", 1)[0].startswith(COOKIES_IGNORADOS))


def pick_cookie(cap):
    for q in reversed(cap.get("reqs", [])):
        for field in (q.get("encKeyAtRequest"), q.get("encKeyAtSend")):
            c = (field or {}).get("cookie")
            if c:
                return prune_cookie(c)
    for q in reversed(cap.get("reqs", [])):
        c = hget(q.get("headers") or {}, "Cookie")
        if c:
            return prune_cookie(c)
    if os.path.exists("cookie.txt"):
        return prune_cookie(open("cookie.txt").read().strip())
    raise SystemExit("sem cookies na captura (e sem cookie.txt)")


def pick_access_uri(cap):
    """O app usa `uri = location.href` no instante em que o service nasce."""
    for q in reversed(cap.get("reqs", [])):
        h = q.get("headers") or {}
        if h.get("Access-Signature") and h.get("Access-Uri"):
            return h["Access-Uri"]
    return FALLBACK_URI


# Headers que o browser envia e que NAO entram na assinatura, mas que o
# antifraude (DataDome) e o tracing do banco exigem: sem eles a autorizacao
# passa e o pedido morre na logica com RESPONSE_CODE=2
# ("Algo inesperado aconteceu"). `x-dtpc` e do DataDome.
EXTRA_HEADERS = ("tracestate", "traceparent", "x-dtpc")


def pick_extra_headers(cap):
    got = {}
    for q in reversed(cap.get("reqs", [])):
        h = q.get("headers") or {}
        for name in EXTRA_HEADERS:
            v = hget(h, name)
            if v and name not in got:
                got[name] = v
        if len(got) == len(EXTRA_HEADERS):
            break
    return got


def main():
    cap, trn, seqarg, datastr, out = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4], sys.argv[5]
    seq = None if seqarg.lower() in ("none", "-1", "null") else int(seqarg)
    data = json.loads(datastr)

    d = json.load(open(cap))
    token = client_id = None
    for q in reversed(d["reqs"]):
        h = q.get("headers") or {}
        if h.get("Authorization") and h.get("Client-Id"):
            token = h["Authorization"].split(" ", 1)[1]
            client_id = h["Client-Id"]
            break
    if not token:
        raise SystemExit("sem token na captura")

    public_rsa, key_src = pick_key(d)
    cookie = pick_cookie(d)
    uri = pick_access_uri(d)
    extras = pick_extra_headers(d)

    body = {"fi": 7, "chn": "3", "trn": trn}
    if seq is not None:
        body["seqFlow"] = seq
    body["data"] = data
    body_s = unicodedata.normalize("NFD", json.dumps(body, separators=(",", ":"), ensure_ascii=False))

    access_id = str(uuid.uuid4())

    priv = ec.generate_private_key(ec.SECP256R1())
    rsa = serialization.load_der_public_key(base64.b64decode(public_rsa))
    spki = base64.b64encode(priv.public_key().public_bytes(
        serialization.Encoding.DER, serialization.PublicFormat.SubjectPublicKeyInfo)).decode()

    def build(headers, atime):
        canon = f"{access_id}:{atime}:{BASE}:{uri}:{body_s}:Bearer {token}"
        der = priv.sign(canon.encode(), ec.ECDSA(hashes.SHA256()))
        ct = rsa.encrypt(json.dumps(spki).encode(), padding.OAEP(
            mgf=padding.MGF1(hashes.SHA1()), algorithm=hashes.SHA256(), label=None))
        h = list(headers) + [
            {"name": "Access-Id", "value": access_id},
            {"name": "Access-Time", "value": atime},
            {"name": "Access-Origin", "value": BASE},
            {"name": "Access-Uri", "value": uri},
            {"name": "Access-Signature", "value": base64.b64encode(der).decode()},
            {"name": "X-Request", "value": base64.b64encode(ct).decode()},
        ]
        return h, canon

    def fetch_ts():
        import http.client
        c = http.client.HTTPSConnection("contaonline.viacredi.coop.br", timeout=20)
        c.request("GET", "/servcorews/getSystemTS", headers={
            "Accept": "application/json, text/plain, */*", "Cookie": cookie, "User-Agent": UA})
        r = c.getresponse()
        return r.read().decode().strip()

    base_headers = [
        {"name": "Host", "value": "contaonline.viacredi.coop.br"},
        {"name": "User-Agent", "value": UA},
        {"name": "Accept", "value": "application/json, text/plain, */*"},
        {"name": "Accept-Language", "value": "pt-BR,pt;q=0.8,en-US;q=0.5,en;q=0.3"},
        {"name": "Content-Type", "value": "application/json"},
        {"name": "Authorization", "value": "Bearer " + token},
        {"name": "Client-Id", "value": client_id},
        {"name": "Origin", "value": BASE},
        {"name": "Referer", "value": BASE + "/ib/dashboard"},
        {"name": "Sec-Fetch-Dest", "value": "empty"},
        {"name": "Sec-Fetch-Mode", "value": "cors"},
        {"name": "Sec-Fetch-Site", "value": "same-origin"},
        {"name": "Cookie", "value": cookie},
        {"name": "Connection", "value": "close"},
    ]
    for name, v in extras.items():
        base_headers.append({"name": name, "value": v})

    atime = fetch_ts()
    headers, canon = build(base_headers, atime)

    spec = {"url": BASE + PATH, "method": "POST", "headers": headers, "body": body_s}
    json.dump(spec, open(out, "w"), indent=2, ensure_ascii=False)

    # O prover commita o pedido em MPC: o record layer aborta acima de 4096+32.
    size = sum(len(h["name"]) + len(h["value"]) + 4 for h in headers) + len(body_s) + 4
    print(f"OK -> {out}")
    print(f"  chave RSA : {key_src}  [{public_rsa[:32]}...]")
    print(f"  Access-Uri: {uri}")
    print(f"  Access-Time={atime}  Access-Id={access_id}")
    print(f"  body={body_s}")
    print(f"  tamanho ~{size} bytes (teto do prover: 4128){'  <<< ESTOURA' if size > 4100 else ''}")


if __name__ == "__main__":
    main()
