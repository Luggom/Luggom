#!/usr/bin/env python3
"""Adapter BB (Bloco 6.4) — gera o LUGGOM_REQUEST_FILE do prover TLSNotary.

Route-agnóstico de propósito: descobre sozinho o pedido do comprovante na
captura (não depende de adivinharmos a rota). O host e a forma do pedido estão
confirmados por captura real (2026-10-28) — ver abaixo.

ANTES de investir no adapter, SONDAR o rail (§13.137-B):
    python3 verify_rail.py --banco banco_do_brasil
Se der FAIL, o BB entra na lista da Caixa e este builder não serve para nada.

Host confirmado por captura real (2026-10-28, Internet Banking PF). O rail
PASSOU a sonda (`verify_rail.py` -> 200 a um cliente TLS independente), logo o
BB é Viacredi-like e o adaptador é viável. Nesta captura o comprovante sai em
`POST /aapf/comprovante/<n>-CO.jsp`; um comprovante de arrecadação (imposto)
pode ter rota própria — o `pick()` descobre-a. O resto (cookie httpOnly do jar,
headers de same-origin, tecto de 4096 bytes) é o padrão já provado na Caixa.

Uso: python3 make_prover_request_bb.py <captura.json> [saida.json]
"""
import json
import sys

# Confirmado pela captura de 2026-10-28 (Internet Banking PF). O scaffold
# adivinhava `www.bb.com.br`; a sessão real vive em `www2.bb.com.br`.
ORIGIN = "https://www2.bb.com.br"
UA = "Mozilla/5.0 (X11; Linux x86_64; rv:154.0) Gecko/20100101 Firefox/154.0"

# Pistas para escolher o pedido do comprovante (o mesmo critério do verify_rail).
PISTAS = ["comprovante", "consultar", "extrato", "processJSON", "detalhe"]

# Tecto do record layer do TLSNotary (4096 + 32). O prover commita o pedido
# inteiro em MPC: acima disto, aborta.
TECTO = 4128

# Cookies sem papel na sessão (analytics/telemetria/marketing) — cortados sempre.
# O jar do BB são ~46 cookies (~3,9 KB só de `Cookie`), que estouram o tecto.
# NÃO se cortam os da sessão: JSESSIONID, gas.gcs.sign, aapf.*, os TS* do WAF
# (F5) e o __cf_bm (Cloudflare).
COOKIES_IGNORADOS = (
    "ttcsid", "_pk_", "ADRUM", "_ga", "_gid", "_gat", "_gcl_", "_gali",
    "_fbp", "_clck", "_clsk", "_ttp", "_tt_", "_twpid", "_twsid", "_dc_gtm_",
    "_sfid_", "_evga_",
)

# Último recurso, por ordem, se ainda estourar depois da telemetria. Provado na
# sessão real (2026-10-28): o comprovante responde 200 sem o `gas.gcs.sign`.
COOKIES_ULTIMO_RECURSO = ("gas.gcs.sign",)


def prune_cookie(ck, extra=()):
    """Corta a telemetria e os cookies de `extra` (por nome exato)."""
    parts = [p.strip() for p in (ck or "").split(";") if p.strip()]
    return "; ".join(
        p for p in parts
        if p.split("=", 1)[0] not in extra
        and not p.split("=", 1)[0].startswith(COOKIES_IGNORADOS))


def fit_cookie(ck, outros_bytes, tecto=TECTO):
    """Devolve um `Cookie` que caiba no tecto: corta a telemetria e, se ainda
    não chegar, o último recurso. `outros_bytes` = tamanho do resto do pedido
    (headers + corpo), sem o valor do `Cookie`."""
    for i in range(len(COOKIES_ULTIMO_RECURSO) + 1):
        c = prune_cookie(ck, COOKIES_ULTIMO_RECURSO[:i])
        if outros_bytes + len(c) <= tecto:
            return c
    return prune_cookie(ck, COOKIES_ULTIMO_RECURSO)


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

    cookies_raw = cap.get("cookies") or ""
    if not cookies_raw:
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
        {"name": "Cookie", "value": ""},
    ]
    if body:
        hdrs.insert(5, {"name": "Content-Type",
                        "value": "application/x-www-form-urlencoded; charset=UTF-8"})

    # O `Cookie` domina o tamanho: encaixa-o no tecto do prover.
    outros = sum(len(h["name"]) + len(h["value"]) + 4 for h in hdrs) + len(body) + 4
    cookies = fit_cookie(cookies_raw, outros)
    for h in hdrs:
        if h["name"] == "Cookie":
            h["value"] = cookies

    spec = {"url": target, "method": (q.get("method") or "POST").upper(),
            "headers": hdrs, "body": body}
    json.dump(spec, open(out, "w"), indent=2, ensure_ascii=False)

    size = sum(len(h["name"]) + len(h["value"]) + 4 for h in hdrs) + len(body) + 4
    print("OK -> %s" % out)
    print("  pedido   : %s %s" % (spec["method"], target.split("?")[0]))
    print("  corpo    : %r" % body[:60])
    n_all = len([c for c in cookies_raw.split(";") if c.strip()])
    n_keep = len([c for c in cookies.split(";") if c.strip()])
    print("  cookies  : %d de %d (jar do browser, inclui httpOnly; %d de telemetria cortados)"
          % (n_keep, n_all, n_all - n_keep))
    print("  tamanho  : ~%d bytes (tecto do prover: 4128)%s"
          % (size, "  <<< ESTOURA" if size > 4128 else ""))


if __name__ == "__main__":
    main()
