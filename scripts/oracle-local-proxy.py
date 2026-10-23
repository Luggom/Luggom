#!/usr/bin/env python3
"""Proxy local de câmbios para o feeder do oráculo.

Porquê: o feeder (`cmd/luggom-oracle-feeder`) espera cada perna na convenção da
chain — CHF/EUR/GBP como **USD por 1 unidade**, BRL/CAD como **unidades por 1
USD** — e uma **fixing date** por resposta. As APIs públicas devolvem a moeda por
USD e não trazem a data no formato que a chain usa; o feeder é um `httpSource` de
caminho pontilhado e não converte nada. Este proxy busca as fontes REAIS e serve
cada uma já convertida, em `http://127.0.0.1:9911/<fonte>`.

Fontes:
  - Frankfurter (referências do BCE) e open.er-api.com — independentes entre si;
  - BCB PTAX (Olinda) para o BRL, que só tem esta fonte oficial.

A fixing date devolvida é a MAIS ANTIGA entre a data do câmbio e a do PTAX: só
se rotula como fixing de um dia aquilo que todas as pernas publicaram para esse
dia. Num fim de semana as três reportam a sexta e o feeder não escreve nada.

Uso: python3 scripts/oracle-local-proxy.py &
"""
import datetime as dt
import json
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = 9911
UPSTREAMS = {
    "/frankfurter": "https://api.frankfurter.dev/v1/latest?base=USD&symbols=CHF,EUR,GBP,CAD",
    "/erapi": "https://open.er-api.com/v6/latest/USD",
}
LEGS = ("CHF", "EUR", "GBP", "CAD")  # CAD não é invertido; as europeias são

PTAX_URL = (
    "https://olinda.bcb.gov.br/olinda/servico/PTAX/versao/v1/odata/"
    "CotacaoDolarPeriodo(dataInicial=@dataInicial,dataFinalCotacao=@dataFinalCotacao)"
    "?@dataInicial='{ini}'&@dataFinalCotacao='{fim}'&$top=100&$format=json"
    "&$select=cotacaoVenda,dataHoraCotacao"
)


def _get(url):
    req = urllib.request.Request(url, headers={"User-Agent": "luggom-oracle-proxy/0.2"})
    with urllib.request.urlopen(req, timeout=15) as resp:
        return json.load(resp)


def fetch_ptax():
    """PTAX de fecho mais recente: devolve (data ISO, BRL por 1 USD)."""
    hoje = dt.datetime.now(dt.timezone.utc).date()
    doc = _get(PTAX_URL.format(
        ini=(hoje - dt.timedelta(days=10)).strftime("%m-%d-%Y"),
        fim=hoje.strftime("%m-%d-%Y"),
    ))
    rows = doc.get("value") or []
    if not rows:
        raise RuntimeError("BCB PTAX sem cotações nos últimos 10 dias")
    last = rows[-1]
    dia = last["dataHoraCotacao"][:10]  # "2026-10-22 13:08:17.041224"
    return dia, float(last["cotacaoVenda"])


def fetch_rates(path):
    """Devolve a fixing date e as 5 pernas na convenção da chain."""
    doc = _get(UPSTREAMS[path])
    rates = doc["rates"]

    if path == "/frankfurter":
        fx_date = doc["date"]  # "2026-10-22"
    else:
        # er-api: "Thu, 22 Oct 2026 00:02:31 +0000" — a data do último update.
        fx_date = dt.datetime.strptime(
            doc["time_last_update_utc"], "%a, %d %b %Y %H:%M:%S %z"
        ).date().isoformat()

    brl_date, brl = fetch_ptax()

    # Só se rotula o dia que TODAS as pernas publicaram.
    fixing = min(fx_date, brl_date)

    out = {
        # rates[X] = X por 1 USD  ->  USD por 1 X = 1 / rates[X]
        "CHF": 1.0 / float(rates["CHF"]),
        "EUR": 1.0 / float(rates["EUR"]),
        "GBP": 1.0 / float(rates["GBP"]),
        # CAD e BRL ficam como estão: unidades por 1 USD.
        "CAD": float(rates["CAD"]),
        "BRL": brl,
    }
    return fixing, out


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path not in UPSTREAMS:
            self.send_response(404)
            self.end_headers()
            return
        try:
            fixing, rates = fetch_rates(self.path)
            body = json.dumps({"date": fixing, "rates": rates}).encode()
            self.send_response(200)
        except Exception as exc:  # fonte caiu -> o feeder descarta-a
            body = json.dumps({"error": str(exc)}).encode()
            self.send_response(502)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    print(f"proxy do oráculo em http://127.0.0.1:{PORT} (fontes: {', '.join(UPSTREAMS)})")
    HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
