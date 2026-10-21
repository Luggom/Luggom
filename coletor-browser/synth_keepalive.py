#!/usr/bin/env python3
"""Como synth_request, mas faz getSystemTS e o POST na MESMA conexao TLS."""
import http.client
import json
import sys
import base64
import unicodedata
import uuid

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, padding

BASE = "contaonline.viacredi.coop.br"
ORIGIN = "https://" + BASE
URI = ORIGIN + "/ib/auth"
UA = "Mozilla/5.0 (X11; Linux x86_64; rv:154.0) Gecko/20100101 Firefox/154.0"


def load(cap, xrf, ckf):
    d = json.load(open(cap))
    for q in reversed(d["reqs"]):
        h = q.get("headers") or {}
        if h.get("Authorization") and h.get("Client-Id"):
            return (h["Authorization"].split(" ", 1)[1], h["Client-Id"])
    raise SystemExit("sem token na captura")


def main():
    cap, xrf, ckf = sys.argv[1], sys.argv[2], sys.argv[3]
    trn = int(sys.argv[4]); seq = None if sys.argv[5] in ("none", "-1") else int(sys.argv[5])
    data = json.loads(sys.argv[6])
    token, client_id = load(cap, xrf, ckf)
    xresp = open(xrf).read().strip()
    cookie = open(ckf).read().strip()

    conn = http.client.HTTPSConnection(BASE, timeout=30)
    hdr = {"Accept": "application/json, text/plain, */*", "Cookie": cookie, "User-Agent": UA}
    conn.request("GET", "/servcorews/getSystemTS", headers=hdr)
    r = conn.getresponse()
    atime = r.read().decode().strip()
    print("getSystemTS:", atime, "| socket reusado:", r.will_close is False)

    body = {"fi": 7, "chn": "3", "trn": trn}
    if seq is not None:
        body["seqFlow"] = seq
    body["data"] = data
    body_s = unicodedata.normalize("NFD", json.dumps(body, separators=(",", ":"), ensure_ascii=False))
    aid = str(uuid.uuid4())

    canon = f"{aid}:{atime}:{ORIGIN}:{URI}:{body_s}:Bearer {token}"
    priv = ec.generate_private_key(ec.SECP256R1())
    der = priv.sign(canon.encode(), ec.ECDSA(hashes.SHA256()))
    spki = base64.b64encode(priv.public_key().public_bytes(
        serialization.Encoding.DER, serialization.PublicFormat.SubjectPublicKeyInfo)).decode()
    rsa = serialization.load_der_public_key(base64.b64decode(xresp))
    mode = sys.argv[7] if len(sys.argv) > 7 else "quoted"
    if mode == "raw":
        pt = spki.encode()
    elif mode == "jwk":
        import json as _j
        nums = priv.public_key().public_numbers()
        b = lambda i: base64.urlsafe_b64encode(i.to_bytes(32, "big")).rstrip(b"=").decode()
        pt = _j.dumps({"kty": "EC", "crv": "P-256", "x": b(nums.x), "y": b(nums.y)}).encode()
    else:
        pt = json.dumps(spki).encode()
    ct = rsa.encrypt(pt, padding.OAEP(
        mgf=padding.MGF1(hashes.SHA1()), algorithm=hashes.SHA256(), label=None))
    print(f"X-Request plaintext mode={mode} len={len(pt)}")

    h2 = {
        "Accept": "application/json, text/plain, */*",
        "Content-Type": "application/json",
        "Authorization": "Bearer " + token,
        "Client-Id": client_id,
        "Access-Id": aid,
        "Access-Time": atime,
        "Access-Origin": ORIGIN,
        "Access-Uri": URI,
        "Access-Signature": base64.b64encode(der).decode(),
        "X-Request": base64.b64encode(ct).decode(),
        "Origin": ORIGIN,
        "Referer": ORIGIN + "/ib/dashboard",
        "User-Agent": UA,
        "Accept-Language": "pt-BR,pt;q=0.8,en-US;q=0.5,en;q=0.3",
        "Sec-Fetch-Dest": "empty", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Site": "same-origin",
        "Cookie": cookie,
    }
    conn.request("POST", "/servcorews/processJSON", body=body_s.encode(), headers=h2)
    r2 = conn.getresponse()
    print("HTTP", r2.status)
    print(r2.read().decode()[:400])
    conn.close()


if __name__ == "__main__":
    main()
