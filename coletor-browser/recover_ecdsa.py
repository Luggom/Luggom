#!/usr/bin/env python3
"""
ECDSA public-key recovery sobre as assinaturas capturadas do navegador.

Se o navegador reutiliza a MESMA chave efemera para varias assinaturas, entao a
chave publica recuperada de (r,s,hash(canonica)) tem de ser IDENTICA em todas.
Isso prova, de uma vez:
  (a) o esquema e ECDSA P-256/SHA-256, e
  (b) a string canonica que reconstruimos e exatamente a que foi assinada
      (se a mensagem estivesse errada, a chave recuperada nao bateria entre
       assinaturas diferentes).

Uso: python3 recover_ecdsa.py [captura.json]
"""
import base64
import hashlib
import json
import sys
from itertools import count

# secp256r1
P = 0xFFFFFFFF00000001000000000000000000000000FFFFFFFFFFFFFFFFFFFFFFFF
A = P - 3
B = 0x5AC635D8AA3A93E7B3EBBD55769886BC651D06B0CC53B0F63BCE3C3E27D2604B
N = 0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551
GX = 0x6B17D1F2E12C4247F8BCE6E563A440F277037D812DEB33A0F4A13945D898C296
GY = 0x4FE342E2FE1A7F9B8EE7EB4A7C0F9E162BCE33576B315ECECBB6406837BF51F5
G = (GX, GY)


def inv(a, m=P):
    return pow(a, -1, m)


def add(p1, p2):
    if p1 is None:
        return p2
    if p2 is None:
        return p1
    x1, y1 = p1
    x2, y2 = p2
    if x1 == x2 and (y1 + y2) % P == 0:
        return None
    if p1 == p2:
        lam = (3 * x1 * x1 + A) * inv(2 * y1) % P
    else:
        lam = (y2 - y1) * inv(x2 - x1) % P
    x3 = (lam * lam - x1 - x2) % P
    y3 = (lam * (x1 - x3) - y1) % P
    return (x3, y3)


def mul(k, pt):
    r = None
    while k:
        if k & 1:
            r = add(r, pt)
        pt = add(pt, pt)
        k >>= 1
    return r


def on_curve(pt):
    if pt is None:
        return False
    x, y = pt
    return (y * y - (x * x * x + A * x + B)) % P == 0


def parse_der_sig(der):
    if len(der) == 64:
        return int.from_bytes(der[:32], "big"), int.from_bytes(der[32:], "big")
    if der[0] != 0x30:
        raise ValueError(f"formato desconhecido ({len(der)} bytes)")
    i = 2
    if der[1] & 0x80:
        i = 2 + (der[1] & 0x7F)
    assert der[i] == 0x02
    rl = der[i + 1]
    r = int.from_bytes(der[i + 2:i + 2 + rl], "big")
    j = i + 2 + rl
    assert der[j] == 0x02
    sl = der[j + 1]
    s = int.from_bytes(der[j + 2:j + 2 + sl], "big")
    return r, s


def recover(r, s, e):
    """Candidatos de chave publica Q para (r,s,e)."""
    out = []
    for j in count(0):
        x = r + j * N
        if x >= P:
            break
        # y par e impar
        alpha = (pow(x, 3, P) + A * x + B) % P
        y = pow(alpha, (P + 1) // 4, P)
        if (y * y - alpha) % P != 0:
            continue
        for yy in {y, (-y) % P}:
            R = (x, yy)
            if not on_curve(R):
                continue
            rinv = inv(r, N)
            Q = mul(rinv, add(mul(s, R), mul((-e) % N, G)))
            if Q and on_curve(Q):
                out.append(Q)
    return out


def main():
    path = sys.argv[1] if len(sys.argv) > 1 else "/tmp/luggom-captura.json"
    d = json.load(open(path))
    signs = [s for s in d.get("signs", []) if s.get("signature") and s.get("canonical")]
    print(f"assinaturas: {len(signs)}")
    seen = {}
    for i, s in enumerate(signs):
        try:
            der = base64.b64decode(s["signature"])
            r, sval = parse_der_sig(der)
        except Exception as ex:
            print(f"  #{i}: assinatura ilegivel ({ex})")
            continue
        e = int.from_bytes(hashlib.sha256(s["canonical"].encode()).digest(), "big")
        cands = recover(r, sval, e)
        if not cands:
            print(f"  #{i}: sem candidatos")
            continue
        # normaliza candidatos
        keys = [f"{q[0]:064x}{q[1]:064x}" for q in cands]
        tag = keys[0][:24]
        seen.setdefault(tuple(sorted(keys)), []).append(i)
        print(f"  #{i}: {len(cands)} candidato(s) Qx={tag}... canon={s['canonical'][:60]!r}")
    print()
    print(f"chaves distintas recuperadas: {len(seen)}")
    for k, idxs in seen.items():
        print(f"  Qx={k[0][:32]}...  aparece em {len(idxs)} assinaturas: {idxs[:12]}{'...' if len(idxs)>12 else ''}")
    if len(seen) == 1:
        print("\n=> UMA unica chave publica para TODAS as assinaturas:")
        print("   (1) ECDSA P-256/SHA-256 CONFIRMADO;")
        print("   (2) nossa string canonica e EXATAMENTE a assinada;")
        print("   (3) o navegador REUTILIZA a mesma chave efemera na sessao.")
    else:
        print("\n=> Chaves diferentes por assinatura: chave efemera por request (ou canonical divergente nalguns).")


if __name__ == "__main__":
    main()
