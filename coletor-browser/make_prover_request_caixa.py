#!/usr/bin/env python3
"""Adapter CAIXA — gera o LUGGOM_REQUEST_FILE do prover TLSNotary.

A Caixa é o OPOSTO da Viacredi/Ailos: REST JSON autenticado por COOKIE, SEM
assinatura (não há X-Request nem Access-Signature nem WebCrypto) — ver §13.127.
Portanto não há nada para assinar: o que autentica o pedido é o cookie de sessão.

E o cookie da Caixa é httpOnly, logo o content script (que só vê
`document.cookie`) NÃO o apanha. Tem de vir do campo `cookies` da captura, que
o background da extensão preenche com a permissão `cookies` (manifest.json).

Uso: python3 make_prover_request_caixa.py <captura.json> <detalhe|comprovante> [saida.json]
"""
import json
import os
import sys

ORIGIN = "https://internetbanking.caixa.gov.br"
UA = ("Mozilla/5.0 (X11; Linux x86_64; rv:154.0) Gecko/20100101 Firefox/154.0")
ROTAS = {
    # POST com corpo `id=<E2E>` — devolve o detalhe estruturado da transacção.
    "detalhe": "detalheExtrato",
    # POST sem corpo — devolve a pagina do comprovante.
    "comprovante": "comprovantePixExtratoSucesso",
    # Extrato de conta (lancamentos) — e onde aparece o debito de convenio/Sicov.
    "extrato": "consulta-extratos",
}


def pick(cap, alvo):
    """Ultimo pedido da rota (o mais recente) + o id E2E que ele usa."""
    best = None
    for q in cap.get("reqs", []):
        u = q.get("url") or ""
        if alvo in u:
            best = q
    if not best:
        raise SystemExit("nao encontrei nenhum pedido para a rota %r" % alvo)
    return best


def cookie_value(cookies, name):
    """Valor de um cookie do jar (string 'a=1; b=2')."""
    for part in cookies.split(";"):
        k, _, v = part.strip().partition("=")
        if k == name:
            return v
    return ""


def main():
    if len(sys.argv) < 3:
        raise SystemExit(__doc__)
    cap_path, rota = sys.argv[1], sys.argv[2]
    out = sys.argv[3] if len(sys.argv) > 3 else "/tmp/luggom-caixa-request.json"

    cap = json.load(open(cap_path))
    alvo = ROTAS.get(rota)
    if not alvo:
        raise SystemExit("rota desconhecida: %r (usa: %s)" % (rota, " | ".join(ROTAS)))

    q = pick(cap, alvo)
    url = q.get("url") or ""
    path = url.split("?")[0]
    # `nocache` e só um cache-buster: cai fora. Restantes params ficam.
    params = [p for p in (url.split("?")[1].split("&") if "?" in url else []) if not p.startswith("nocache=")]
    target = ORIGIN + path + ("?" + "&".join(params) if params else "")
    body = q.get("body") or ""

    # O cookie TEM de vir do jar (httpOnly), não do content script.
    cookies = cap.get("cookies") or ""
    if not cookies:
        det = cap.get("cookiesDetail")
        raise SystemExit(
            "a captura nao traz `cookies` (o background da extensao so os le com a "
            "permissao `cookies` + host_permissions do banco). cookiesDetail=%r" % (det,))

    hdrs = [
        {"name": "Host", "value": "internetbanking.caixa.gov.br"},
        {"name": "User-Agent", "value": UA},
        {"name": "Accept", "value": "application/json, text/plain, */*"},
        {"name": "Accept-Language", "value": "pt-BR,pt;q=0.8,en-US;q=0.5,en;q=0.3"},
        {"name": "X-Requested-With", "value": "XMLHttpRequest"},
        # O colector só captura headers definidos pelo JS; o Referer/Origin e os
        # Sec-Fetch-* que o browser acrescenta não vêm. Reconstroem-se como o
        # Firefox os manda (o Sec-Fetch-* faltava e podia ser o que o WAF olha).
        # NÃO se manda Accept-Encoding: comprimiria a resposta e o extractor
        # deixaria de ver o corpo.
        {"name": "Sec-Fetch-Dest", "value": "empty"},
        {"name": "Sec-Fetch-Mode", "value": "cors"},
        {"name": "Sec-Fetch-Site", "value": "same-origin"},
        {"name": "Referer", "value": ORIGIN + "/sinbc/"},
        {"name": "Origin", "value": ORIGIN},
        {"name": "Connection", "value": "close"},
        {"name": "Cookie", "value": cookies},
    ]
    if body:
        hdrs.insert(5, {"name": "Content-Type", "value": "application/x-www-form-urlencoded; charset=UTF-8"})

    # DataDome (o antifraude da Caixa — o jar tem dtCookie/dtPC/dtSa): o cliente
    # do banco manda `x-dtpc` com o valor do cookie `dtPC`, e o tracing manda
    # `tracestate`/`traceparent`. A Viacredi precisava EXACTAMENTE destes três
    # (ver make_prover_request.py: "sem eles a autorizacao passa e o pedido morre
    # na logica"). Aqui faltavam — são o primeiro candidato.
    dtpc = cookie_value(cookies, "dtPC")
    if dtpc:
        hdrs.append({"name": "x-dtpc", "value": dtpc})
    for extra in ("tracestate", "traceparent"):
        v = None
        for k, val in (q.get("headers") or {}).items():
            if k.lower() == extra:
                v = val
        if v:
            hdrs.append({"name": extra, "value": v})

    spec = {"url": target, "method": (q.get("method") or "POST").upper(), "headers": hdrs, "body": body}
    json.dump(spec, open(out, "w"), indent=2, ensure_ascii=False)

    size = sum(len(h["name"]) + len(h["value"]) + 4 for h in hdrs) + len(body) + 4
    n_ck = len(cookies.split(";"))
    print("OK -> %s" % out)
    print("  rota     : %s" % rota)
    print("  alvo     : %s" % target)
    print("  metodo   : %s | corpo: %r" % (spec["method"], body[:60]))
    print("  cookies  : %d (jar do browser, inclui httpOnly)" % n_ck)
    print("  tamanho  : ~%d bytes (teto do prover: 4128)%s" % (size, "  <<< ESTOURA" if size > 4100 else ""))


if __name__ == "__main__":
    main()
