#!/usr/bin/env python3
"""Imprime a FORMA de uma captura do coletor — nunca os valores.

Porquê: para construir o extractor de um rail é preciso saber os NOMES dos
campos, o formato da data, o formato do valor e qual é o identificador canónico.
Não é preciso — nem se deve — enviar o comprovante com o nome e o CPF de alguém.

O que mostra:
  - `pageUrl` e contagens (signs/reqs/resps);
  - os pedidos e respostas do host do banco (método, path, status, tamanho);
  - para corpos JSON, a ÁRVORE de chaves com o TIPO de cada valor e, para
    strings, apenas o comprimento + a classe (data, valor, id fiscal, hash,
    e2e) — o conteúdo fica mascarado.

Uso:
  python3 inspect_capture.py /tmp/luggom-captura.json
  python3 inspect_capture.py /tmp/luggom-captura.json --host bb.com.br
"""
import json
import re
import sys

DEFAULT = "/tmp/luggom-captura.json"
MAX_DEPTH = 6
MAX_ITEMS = 8

RE_DATE = re.compile(r"^\s*\d{2}/\d{2}/\d{4}(\s+\d{2}:\d{2}(:\d{2})?)?\s*$")
RE_DATE_ISO = re.compile(r"^\s*\d{4}-\d{2}-\d{2}([T ]\d{2}:\d{2}(:\d{2})?)?")
RE_MONEY = re.compile(r"^\s*(R\$|US\$|C\$|\$)?\s*[\d.]*\d([.,]\d{1,2})?\s*$")
RE_E2E = re.compile(r"^E\d{8}[0-9A-Z]{10,}$")
RE_HEX = re.compile(r"^[0-9a-fA-F]{32,}$")
RE_FISCAL = re.compile(r"^[\d./-]{11,20}$")


def classify(v):
    """Classe do valor + comprimento. Nunca devolve o valor."""
    s = str(v)
    if RE_DATE.match(s):
        return "date(%d)" % len(s)
    if RE_DATE_ISO.match(s):
        return "date-iso(%d)" % len(s)
    if RE_E2E.match(s):
        return "pix-e2e(%d)" % len(s)
    if RE_HEX.match(s):
        return "hex(%d)" % len(s)
    if RE_MONEY.match(s) and any(c.isdigit() for c in s):
        return "money(%d)" % len(s)
    if RE_FISCAL.match(s):
        return "fiscal-id(%d)" % len(s)
    return "str(%d)" % len(s)


def walk(node, prefix="", depth=0):
    if depth > MAX_DEPTH:
        print("%s… (profundidade máxima)" % prefix)
        return
    if isinstance(node, dict):
        for i, (k, v) in enumerate(node.items()):
            if i >= MAX_ITEMS:
                print("%s%s… (%d chaves no total)" % (prefix, "", len(node)))
                break
            if isinstance(v, (dict, list)):
                print("%s%s:" % (prefix, k))
                walk(v, prefix + "  ", depth + 1)
            else:
                print("%s%s = %s" % (prefix, k, classify(v)))
    elif isinstance(node, list):
        if not node:
            print("%s[] (vazio)" % prefix)
            return
        print("%s[%d itens; forma do primeiro]" % (prefix, len(node)))
        walk(node[0], prefix + "  ", depth + 1)
    else:
        print("%s= %s" % (prefix, classify(node)))


def main():
    argv = sys.argv[1:]
    host = ""
    if "--host" in argv:
        i = argv.index("--host")
        host = (argv[i + 1] if i + 1 < len(argv) else "").lower()
        argv = argv[:i] + argv[i + 2:]
    path = argv[0] if argv else DEFAULT

    with open(path) as f:
        cap = json.load(f)

    reqs = cap.get("reqs") or []
    resps = cap.get("resps") or []
    print("captura : %s" % path)
    print("pageUrl : %s" % (cap.get("pageUrl") or "(sem pageUrl — extensão antiga?)"))
    print("signs=%d reqs=%d resps=%d cookies=%s" % (
        len(cap.get("signs") or []), len(reqs), len(resps),
        "sim" if cap.get("cookies") else "NAO"))
    print("=" * 70)

    def matches(url):
        url = url or ""
        # Os pedidos vêm com PATH relativo (a origem está no pageUrl): mostram-se
        # sempre, senão o filtro por host escondia-os todos.
        if not url.startswith("http"):
            return True
        return (not host) or (host in url)

    print("\n[1] pedidos do host")
    for q in reqs:
        if matches(q.get("url")):
            body = q.get("body") or ""
            print("  %-6s %-60s corpo=%d" % (
                (q.get("method") or "GET").upper(), (q.get("url") or "")[:60], len(body)))

    print("\n[2] respostas do host (forma do corpo)")
    for r in resps:
        if not matches(r.get("url")):
            continue
        body = r.get("body") or ""
        print("\n  -> %s  status=%s  corpo=%d" % (
            (r.get("url") or "")[:70], r.get("status", "?"), len(body)))
        try:
            doc = json.loads(body)
        except Exception:
            print("     (não é JSON — se for HTML, copia só os RÓTULOS dos campos)")
            continue
        walk(doc, "     ")


if __name__ == "__main__":
    main()
