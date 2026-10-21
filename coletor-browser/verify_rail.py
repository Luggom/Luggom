#!/usr/bin/env python3
"""SONDA DE RAIL — o rail de um banco aceita um cliente TLS INDEPENDENTE?

PORQUÊ (a lição do §13.137-B): antes de construir o adapter de um banco, tem de
se saber se o pedido do comprovante responde a um cliente que NÃO é o browser.
A Caixa **falha**: 302 `sessaoEncerradaNovo` em TODOS os perfis testados
(Firefox 147/148 — o mais novo que existe —, **Chrome 150 atual**, Safari 260),
com os headers e cookies exactos, enquanto o Firefox 154 do utilizador responde
200. Gastaram-se sessões a construir o adapter da Caixa para um rail que não
passa. **Ordem correcta: capturar → sondar → só depois construir.**

O que faz: repete o pedido capturado com `curl_cffi` a impersonar o Firefox
(tenta do perfil mais NOVO para o mais antigo), com os cookies do jar (inclui os
httpOnly) e os headers de uma chamada same-origin da página.

Leitura do resultado:
  PASS  200 ou 400   -> o rail responde a um cliente independente (400 = faltam
                        parâmetros, mas o cliente PASSOU). Construir o adapter.
  302 .../sessaoEncerradaNovo (em todas as variantes)
        -> o WAF exige o próprio browser. NÃO construir adapter — repetir a
           história da Caixa. Ver §13.137-B antes de insistir.
  outro -> ler o corpo; resultado inesperado.

Requer `curl_cffi` (não vem com o Python do sistema):
  sudo apt install -y python3-pip && pip install --user --break-system-packages curl_cffi

Uso:
  python3 verify_rail.py                       # captura mais recente de runs/
  python3 verify_rail.py <captura.json>
  python3 verify_rail.py --banco bb --dry-run
"""
import glob
import json
import os
import sys
import time

# Pistas para PRIORIZAR o pedido certo (não são obrigatórias: qualquer POST
# same-origin serve para sondar o rail — o que se testa é o CLIENTE).
PISTAS = [
    "consulta-extratos",           # Caixa: extrato
    "comprovantePixExtratoSucesso",  # Caixa: comprovante PIX
    "detalheExtrato",              # Caixa: detalhe da transacção
    "processJSON",                 # Ailos/Viacredi: consulta assinada
    "comprovante",                 # genérico
]

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
RUNS_DIR = os.path.join(SCRIPT_DIR, "runs")


def newest_capture():
    """A captura mais recente com cookies do jar e pedidos (ignora vazias)."""
    best = None
    for path in glob.glob(os.path.join(RUNS_DIR, "**", "captura*.json"), recursive=True):
        try:
            with open(path) as f:
                cap = json.load(f)
        except Exception:
            continue
        if not cap.get("cookies") or not cap.get("reqs"):
            continue
        mt = os.path.getmtime(path)
        if best is None or mt > best[0]:
            best = (mt, path, cap)
    return best


def origin_of(cap, q):
    """Origem da captura, para o pedido `q`.

    Ordem: `pageUrl` (o background acrescenta-o) → a resposta cujo CAMINHO casa
    com o do pedido (é o mesmo recurso, logo a mesma origem). NUNCA o primeiro
    URL absoluto da captura: uma captura real traz trackers de terceiros e
    apanharia o host errado.
    """
    page = cap.get("pageUrl") or ""
    if page.startswith("http"):
        return "/".join(page.split("/")[:3])
    want = (q.get("url") or "").split("?")[0]
    for e in (cap.get("resps") or []) + (cap.get("reqs") or []):
        u = e.get("url") or ""
        if u.startswith("http") and u.split("?")[0].endswith(want):
            return "/".join(u.split("/")[:3])
    raise SystemExit(
        "não consegui inferir a ORIGEM do pedido %r: sem `pageUrl` e sem "
        "nenhuma resposta com o mesmo caminho. Recaptura com a extensão "
        "actualizada (background.js acrescenta `pageUrl`)." % want)


def pick_request(cap):
    """O pedido a repetir: o último que casa uma pista; senão o último POST."""
    reqs = cap.get("reqs") or []
    for pista in PISTAS:
        matches = [q for q in reqs if pista in (q.get("url") or "")]
        if matches:
            return matches[-1]
    posts = [q for q in reqs if (q.get("method") or "").upper() == "POST"]
    if posts:
        return posts[-1]
    raise SystemExit("a captura não tem nenhum POST para repetir")


def build_url(origin, q):
    """ORIGIN + caminho, sem o cache-buster `nocache` (se existir)."""
    url = q.get("url") or ""
    if url.startswith("http"):
        return url
    path = url.split("?")[0]
    params = [p for p in (url.split("?")[1].split("&") if "?" in url else [])
              if not p.startswith("nocache=")]
    return origin + path + ("?" + "&".join(params) if params else "")


def main():
    args = [a for a in sys.argv[1:] if not a.startswith("-")]
    banco = ""
    argv = sys.argv[1:]
    if "--banco" in argv:
        i = argv.index("--banco")
        if i + 1 < len(argv):
            banco = argv[i + 1]
            args = [a for a in args if a != banco]

    if args:
        cap_path = args[0]
        with open(cap_path) as f:
            cap = json.load(f)
    else:
        found = newest_capture()
        if not found:
            raise SystemExit(
                "sem capturas em %s — corre o coletor (collector-server.py) e a "
                "extensão no banco" % RUNS_DIR)
        _, cap_path, cap = found

    age = time.time() - os.path.getmtime(cap_path)
    stale = age > 300
    print("captura : %s  (há %.0f s)" % (cap_path, age))
    if stale:
        print("⚠️  AVISO: captura com mais de 5 min — um FAIL pode ser a SESSÃO "
              "já caída, não o rail. Confirma com uma captura fresca.")

    q = pick_request(cap)
    origin = origin_of(cap, q)
    url = build_url(origin, q)
    body = q.get("body") or ""
    cookies = cap.get("cookies") or ""
    if not cookies:
        raise SystemExit(
            "a captura não traz `cookies` (precisa da permissão `cookies` + "
            "host_permissions do banco no background da extensão)")

    print("banco   : %s" % (banco or "(inferido) " + origin))
    print("origem  : %s" % origin)
    print("pedido  : %s %s" % ((q.get("method") or "POST").upper(), url.split("?")[0]))
    print("cookies : %d" % len([c for c in cookies.split(";") if c.strip()]))

    if "--dry-run" in sys.argv:
        print("\nDRY-RUN (nada foi enviado).")
        print("alvo        : %s" % url)
        print("corpo       : %r" % body[:80])
        print("headers capt: %s" % ", ".join(sorted((q.get("headers") or {}).keys())))
        return 0

    try:
        from curl_cffi import requests as creq
    except ImportError:
        raise SystemExit(
            "falta o curl_cffi. Instala com:\n"
            "  sudo apt install -y python3-pip && "
            "pip install --user --break-system-packages curl_cffi")

    # Headers de uma chamada same-origin da página. NÃO se fixa User-Agent: é o
    # ClientHello/UA do perfil impersonado que se está a testar.
    headers = {
        "Accept": "application/json, text/plain, */*",
        "Accept-Language": "pt-BR,pt;q=0.8,en-US;q=0.5,en;q=0.3",
        "X-Requested-With": "XMLHttpRequest",
        "Sec-Fetch-Dest": "empty",
        "Sec-Fetch-Mode": "cors",
        "Sec-Fetch-Site": "same-origin",
        "Referer": origin + "/",
        "Origin": origin,
        "Cookie": cookies,
    }
    if body:
        headers["Content-Type"] = "application/x-www-form-urlencoded; charset=UTF-8"

    variantes = ("firefox147", "firefox144", "firefox135", "firefox133", "firefox")
    seen = []
    last_err = None
    for imp in variantes:
        try:
            r = creq.request((q.get("method") or "POST").upper(), url,
                             headers=headers,
                             data=body.encode() if body else None,
                             impersonate=imp, timeout=30, allow_redirects=False)
        except Exception as e:  # noqa: BLE001 — variante indisponível
            last_err = "%s: %s" % (imp, e)
            continue
        loc = r.headers.get("location", "")
        seen.append((imp, r.status_code, loc, len(r.content or b"")))
        print("  %-10s -> %d  corpo=%d  %s" % (imp, r.status_code, len(r.content or b""), loc))

        if r.status_code in (200, 400):
            print("\n=> PASS: o rail responde a um cliente independente (%s)." % imp)
            print("   Adapter pode ser construído.")
            return 0

    if not seen:
        raise SystemExit("curl_cffi falhou em todas as variantes: %s" % last_err)

    bloqueadas = [s for s in seen if s[1] == 302 and "sessaoEncerrada" in s[2]]
    if len(bloqueadas) == len(seen):
        print("\n=> FAIL: bloqueado em TODAS as variantes (302 de fachada).")
        if stale:
            print("   ⚠️  MAS a captura tem %.0f s: confirma com uma FRESCA antes "
                  "de decidir — pode ser só a sessão caída." % age)
        else:
            print("   O WAF exige o próprio browser. NÃO construir o adapter.")
            print("   Caso de referência: a Caixa (§13.137-B) — Firefox 147/148 e")
            print("   Chrome 150 atual falham, com os headers e cookies exactos.")
        return 1

    print("\n=> resultado MISTO: %r" % ([s for s in seen if s not in bloqueadas],))
    return 2


if __name__ == "__main__":
    sys.exit(main())
