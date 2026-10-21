#!/usr/bin/env python3
"""Coletor local da extensao Luggom (recebe POST /collect e grava um arquivo)."""
import json
import os
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

OUT = os.environ.get("LUGGOM_CAPTURA", "/tmp/luggom-captura.json")


class H(BaseHTTPRequestHandler):
    def _cors(self):
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Headers", "*")
        self.send_header("Access-Control-Allow-Methods", "POST, OPTIONS")

    def do_OPTIONS(self):
        self.send_response(204)
        self._cors()
        self.end_headers()

    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        raw = self.rfile.read(n)
        with open(OUT, "wb") as f:
            f.write(raw)
        # Cópia com carimbo de tempo: o content script reinicia a lista a cada
        # documento novo (login/reload), e cada push SUBSTITUI o ficheiro. Sem
        # isto, uma captura boa perde-se quando a página recarrega (já aconteceu).
        try:
            import time
            stamped = OUT + "." + time.strftime("%H%M%S") + ".json"
            with open(stamped, "wb") as f:
                f.write(raw)
        except Exception:
            stamped = "(falhou)"
        try:
            d = json.loads(raw)
            print(f"RECEBIDO: signs={len(d.get('signs', []))} reqs={len(d.get('reqs', []))} resps={len(d.get('resps', []))} -> {OUT}", flush=True)
            if isinstance(stamped, str) and stamped != "(falhou)":
                print(f"          (cópia: {os.path.basename(stamped)})", flush=True)
        except Exception as e:
            print(f"RECEBIDO ({len(raw)} bytes, nao-JSON: {e}) -> {OUT}", flush=True)
        self.send_response(200)
        self._cors()
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"ok":true}')

    def log_message(self, *a):
        pass


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8099
    print(f"Coletor Luggom ouvindo em http://127.0.0.1:{port}/collect (arquivo: {OUT})", flush=True)
    HTTPServer(("127.0.0.1", port), H).serve_forever()
