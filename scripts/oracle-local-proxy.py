#!/usr/bin/env python3
"""Proxy local de câmbios para o feeder do oráculo (hard test).

Porquê: o feeder (`cmd/luggom-oracle-feeder`) espera cada perna como **USD por 1
unidade** (CHF/USD, EUR/USD, GBP/USD) — é essa a convenção da chain
(`x/luggom/keeper/oracle_usd.go`, `minFxLegUSD=0.05`..`maxFxLegUSD=50`). As APIs
públicas devolvem o inverso ("moeda por USD") e o feeder **não inverte** (é um
`httpSource` de caminho pontilhado). Este proxy busca as duas fontes REAIS e
serve cada uma já invertida, em `http://127.0.0.1:9911/<fonte>`.

As duas fontes continuam a ser **independentes** (Frankfurter e
open.er-api.com) — o feeder tira a mediana das duas, como em produção.

Uso: python3 scripts/oracle-local-proxy.py &
"""
import json
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = 9911
UPSTREAMS = {
    "/frankfurter": "https://api.frankfurter.dev/v1/latest?base=USD&symbols=CHF,EUR,GBP",
    "/erapi": "https://open.er-api.com/v6/latest/USD",
}
LEGS = ("CHF", "EUR", "GBP")


def fetch_usd_per_unit(url):
    req = urllib.request.Request(url, headers={"User-Agent": "luggom-oracle-proxy/0.1"})
    with urllib.request.urlopen(req, timeout=10) as resp:
        doc = json.load(resp)
    rates = doc["rates"]
    # rates[X] = X por 1 USD  ->  USD por 1 X = 1 / rates[X]
    return {leg: 1.0 / float(rates[leg]) for leg in LEGS}


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path not in UPSTREAMS:
            self.send_response(404)
            self.end_headers()
            return
        try:
            rates = fetch_usd_per_unit(UPSTREAMS[self.path])
            body = json.dumps({"rates": rates}).encode()
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
