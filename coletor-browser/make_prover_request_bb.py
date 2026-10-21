#!/usr/bin/env python3
"""Adapter BB (Bloco 6.4) — gera o LUGGOM_REQUEST_FILE do prover TLSNotary.

⚠️ SCAFFOLD: NÃO há captura real do BB. Este builder é **route-agnóstico** de
propósito: descobre sozinho o pedido do comprovante na captura, para servir no
dia em que existir uma sessão real (não depende de adivinharmos a rota).

ANTES de investir no adapter, SONDAR o rail (§13.137-B):
    python3 verify_rail.py --banco banco_do_brasil
Se der FAIL, o BB entra na lista da Caixa e este builder não serve para nada.

O que falta confirmar com uma captura real: o host (aqui `www.bb.com.br`), a rota
e o corpo. O resto (cookie httpOnly do jar, headers de same-origin, tecto de
4096 bytes) é o padrão já provado na Caixa.

Uso: python3 make_prover_request_bb.py <captura.json> [saida.json]
"""
import json
import sys

ORIGIN = "https://www.bb.com.br"  # ⚠️ CONFIRMAR com captura real
UA = "Mozilla/5.0 (X11; Linux x86_64; rv:154.0) Gecko/20100101 Firefox/154.0"

# Pistas para escolher o pedido do comprovante (o mesmo critério do verify_rail).
PISTAS = ["comprovante", "consultar", "extrato", "processJSON", "detalhe"]


def pick(cap):
    reqs = cap.get("reqs") or []
    for pista in PISTAS:
        m = [q for q in reqs if pista in (q.get("url") or "")]
        if m:
            return m[-1]
    posts = [q for q in reqs if (q.get("method") or "").upper() == "POST"]
    if posts:
        return posts[-1]
    raise SystemExit("não encontrei nenhum pedido de comprovante na captura")


def main():
    if len(sys.argv) < 2:
        raise SystemExit(__doc__)
    cap_path = sys.argv[1]
    out = sys.argv[2] if len(sys.argv) > 2 else "/tmp/luggom-bb-request.json"

    cap = json.load(open(cap_path))
    q = pick(cap)

    url = q.get("url") or ""
    if url.startswith("http"):
        target = url
    else:
        path = url.split("?")[0]
        params = [p for p in (url.split("?")[1].split("&") if "?" in url else [])
                  if not p.startswith("nocache=")]
        target = ORIGIN + path + ("?" + "&".join(params) if params else "")
    body = q.get("body") or ""

    cookies = cap.get("cookies") or ""
    if not cookies:
        raise SystemExit(
            "a captura não traz `cookies` (precisa da permissão `cookies` + "
            "host_permissions do BB no background da extensão)")

    hdrs = [
        {"name": "Host", "value": ORIGIN.split("//")[1]},
        {"name": "User-Agent", "value": UA},
        {"name": "Accept", "value": "application/json, text/plain, */*"},
        {"name": "Accept-Language", "value": "pt-BR,pt;q=0.8,en-US;q=0.5,en;q=0.3"},
        {"name": "X-Requested-With", "value": "XMLHttpRequest"},
        # Sec-Fetch-* de uma chamada same-origin da página (§13.137-A: a forma
        # dos headers não é o discriminador, mas tem de estar certa).
        # NÃO se manda Accept-Encoding: comprimiria a resposta e o extractor
        # deixaria de ver o corpo (spec §5).
        {"name": "Sec-Fetch-Dest", "value": "empty"},
        {"name": "Sec-Fetch-Mode", "value": "cors"},
        {"name": "Sec-Fetch-Site", "value": "same-origin"},
        {"name": "Referer", "value": ORIGIN + "/"},
        {"name": "Origin", "value": ORIGIN},
        {"name": "Connection", "value": "close"},
        {"name": "Cookie", "value": cookies},
    ]
    if body:
        hdrs.insert(5, {"name": "Content-Type",
                        "value": "application/x-www-form-urlencoded; charset=UTF-8"})

    spec = {"url": target, "method": (q.get("method") or "POST").upper(),
            "headers": hdrs, "body": body}
    json.dump(spec, open(out, "w"), indent=2, ensure_ascii=False)

    size = sum(len(h["name"]) + len(h["value"]) + 4 for h in hdrs) + len(body) + 4
    print("OK -> %s" % out)
    print("  pedido   : %s %s" % (spec["method"], target.split("?")[0]))
    print("  corpo    : %r" % body[:60])
    print("  cookies  : %d (jar do browser, inclui httpOnly)"
          % len([c for c in cookies.split(";") if c.strip()]))
    print("  tamanho  : ~%d bytes (tecto do prover: 4128)%s"
          % (size, "  <<< ESTOURA" if size > 4100 else ""))
    print("  ⚠️ host/rota/corpo por CONFIRMAR (não há captura real do BB)")


if __name__ == "__main__":
    main()
