// Banco sandbox local (Bloco 6.2): serve os FORMATOS dos bancos para se
// desenvolverem e testarem adapters SEM conta real.
//
// Serve:
//   GET  /                                                  — "login" que seta o cookie de sessão
//   GET  /comprovante                                       — recibo HTML IPTU (o original)
//   POST /servcorews/processJSON                            — formato Ailos/Viacredi (JSON OUTPUT_DATA.WS_RESULT)
//   POST /sinbc-saldo/nb/extratos/consulta-extratos          — formato Caixa: extrato
//   POST /sinbc-pix/nb/extrato/comprovantePixExtratoSucesso  — formato Caixa: comprovante de PIX
//   POST /bb/nb/comprovantes/consultar                       — formato BB de REFERÊNCIA (ver abaixo)
//
// Todas as rotas de comprovante exigem o cookie de sessão e devolvem
// X-Receipt-SHA256 (torna o anti-replay reproduzível nos testes).
//
// ⚠️ Os corpos são SINTÉTICOS com os FORMATOS reais — nenhum dado de pessoa. O
// formato BB é uma **REFERÊNCIA NOSSA** (não há recibo BB real; ver
// docs/adapter-spec.md) e está marcado como tal para ninguém o confundir com
// evidência.
//
// ⚠️ O comprovante HTML traz o marcador `data-tax-entity`, que era honrado por
// uma flag de DEV. §13.145 (A10): essa flag foi REMOVIDA do código — o marcador
// já NÃO é honrado em lado nenhum, e o corpo atestado nunca escolhe o ente. As
// rotas resolvem o ente pelo CNPJ mascarado (br:iptu_sao_paulo). O marcador
// ficou no ficheiro como dado inerte.
//
// O túnel (localhost.run/cloudflared) expõe 127.0.0.1:9000 como HTTPS público
// com cert Let's Encrypt — aceito pelo prover TLSNotary (raízes Mozilla).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

// htmlReceipt é o comprovante original (formato genérico BR em HTML).
const htmlReceipt = `<html><body><h1>Comprovante de Pagamento</h1>
<p class="data-tax-entity="br:sefaz_sp"">SEFAZ - Prefeitura do Município</p>
<p>Favorecido: Prefeitura do Município de São Paulo</p>
<p>CNPJ: **392.130/0001-**</p>
<p>Pagador: FULANO DE TAL</p>
<p>Valor: R$ 1.234,56</p>
<p>ID da operação: SBOX20260908123456ABC</p>
<p>Autenticação: 3F2A9C11DD550E88</p>
<p>Tipo: ARRECADAÇÃO IPTU - BR.GOV.BR/AREC</p>
<p>{"data":"08/09/2026 10:11:12"}</p>
</body></html>`

// ailosReceipt reproduz o formato do Ailos/Viacredi: JSON com OUTPUT_DATA.WS_RESULT.
// O valor TOTAL vem antes da multa/juros (é o que evita ler "multa":"R$ 0,00"), a
// data traz hora, e o id do pagamento é o E2E do PIX em `idTransacao`.
const ailosReceipt = `{"data":{"OUTPUT_DATA":{"WS_RESULT":{"recebedorInstituicao":"BCO SAND COOP","recebedorNome":"SECRETARIA MUNICIPAL DA FAZENDA","data":"15/07/2026 18:07:44","multa":"R$ 0,00","valorOriginalPix":"R$ 1.291,49","protocolo":"2306.4030.1801.0F07.1A41.1A29.50","descontos":"R$ 0,00","valorTotal":"R$ 1.291,49","numeroDocumento":"867596174","juros":"R$ 0,00","valor":"R$ 1.291,49","valorTaxa":"R$ 0,00","idTransacao":"E99999991202601010000VC1YWGD3E47","recebedorDocumento":"**.392.130/0001-**"}},"RESPONSE_CODE":0}}`

// caixaExtrato reproduz a forma do extrato da Caixa (lista de lançamentos).
const caixaExtrato = `{"extrato":[{"data":"15/07/2026","descricao":"ARRECADACAO IPTU","valor":"-1.291,49","saldo":"3.708,51"}]}`

// caixaPixReceipt reproduz o comprovante de PIX da Caixa: valor SEM "R$", id
// canónico no campo `id` (o E2E do PIX), estado em `situacaoTransacao`, e o ente
// em `usuarioRecebedor.cnpj` (aqui mascarado → br:iptu_sao_paulo). O pagador é
// pessoa física (`cpf`), logo o portão 1206 não dispara.
const caixaPixReceipt = `{"detalheExtrato":{"E99999991202601010000VC1YWGD3E47":{"id":"E99999991202601010000VC1YWGD3E47","valor":"1.291,49","situacaoTransacao":"CONCLUIDA","data":"17/09/2026 10:11:12","autenticacao":"3F2A9C11DD550E88","descricao":"ARRECADACAO IPTU","usuarioPagador":{"cnpj":"","cpf":"00000000000","conta":{"agencia":null,"chave":"699dfe8a-4649-415f-8fe1-b91460d0fb87"}},"usuarioRecebedor":{"cnpj":"**.392.130/0001-**","cpf":"","conta":{"agencia":null,"chave":"22222222-3333-4444-5555-666666666666"}}}}}`

// bbReceipt é o formato BB **DE REFERÊNCIA** — não é um layout real (não há
// recibo do BB): é a forma que o SPEC do adapter pede a um portal (valor pago
// rotulado, data, beneficiário com o ID do ente, pagador com ID, natureza).
const bbReceipt = `{"comprovante":{"idTransacao":"E99999991202601010000VC1YWGD3E47","data":"15/07/2026 18:07:44","valorPago":"1.291,49","beneficiario":{"nome":"SECRETARIA MUNICIPAL DA FAZENDA","documento":"**.392.130/0001-**"},"pagador":{"nome":"ACME COMERCIO LTDA","documento":"12.345.678/0001-90"},"tipoServico":"ARRECADACAO IPTU","situacao":"PAGO"}}`

type formato struct {
	rota        string
	metodo      string
	contentType string
	corpo       string
	referencia  bool // true = formato nosso (não um layout real de banco)
}

var formatos = []formato{
	{rota: "/comprovante", metodo: http.MethodGet, contentType: "text/html; charset=utf-8", corpo: htmlReceipt},
	{rota: "/servcorews/processJSON", metodo: http.MethodPost, contentType: "application/json", corpo: ailosReceipt},
	{rota: "/sinbc-saldo/nb/extratos/consulta-extratos", metodo: http.MethodPost, contentType: "application/json", corpo: caixaExtrato},
	{rota: "/sinbc-pix/nb/extrato/comprovantePixExtratoSucesso", metodo: http.MethodPost, contentType: "application/json", corpo: caixaPixReceipt},
	{rota: "/bb/nb/comprovantes/consultar", metodo: http.MethodPost, contentType: "application/json", corpo: bbReceipt, referencia: true},
}

func main() {
	port := os.Getenv("SANDBOX_PORT")
	if port == "" {
		port = "9000"
	}
	addr := "127.0.0.1:" + port
	log.Printf("sandbox-bank listening on http://%s", addr)
	if err := http.ListenAndServe(addr, newMux()); err != nil {
		log.Fatal(err)
	}
}

// newMux monta as rotas (separado do main para os testes as exercitarem).
func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", loginHandler)
	for _, f := range formatos {
		mux.HandleFunc(f.rota, receiptHandler(f))
	}
	return mux
}

// loginHandler é a "página de login" de teste: seta o cookie de sessão.
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	target := r.URL.Query().Get("next")
	if target == "" {
		target = "https://" + r.Host + "/comprovante"
	}
	w.Header().Set("Set-Cookie", "sandbox_session=ok; Path=/; HttpOnly")
	fmt.Fprintf(w, `<html><body>
<h1>Banco Sandbox Luggom (login de teste)</h1>
<form method="get" action="%s" style="margin-top:16px">
  <label>Usuário: <input name="u" value="admin"></label><br/><br/>
  <label>Senha: <input type="password" name="p" value="admin"></label><br/><br/>
  <button type="submit">Entrar e ver comprovante</button>
</form>
<p style="font-size:12px;color:#666">Ambiente de teste — nenhum dado real.</p>
</body></html>`, httpEscapeHTML(target))
}

// receiptHandler serve um formato, exigindo o cookie de sessão e assinando o
// corpo com X-Receipt-SHA256 (estável ⇒ anti-replay reproduzível no teste).
func receiptHandler(f formato) http.HandlerFunc {
	sum := sha256.Sum256([]byte(f.corpo))
	sha := hex.EncodeToString(sum[:])
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != f.metodo {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c, err := r.Cookie("sandbox_session")
		if err != nil || c.Value != "ok" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		w.Header().Set("X-Receipt-SHA256", sha)
		w.Header().Set("Content-Type", f.contentType)
		_, _ = w.Write([]byte(f.corpo))
	}
}

func httpEscapeHTML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;", "'", "&#39;", "<", "&lt;", ">", "&gt;", `"`, "&quot;",
	).Replace(s)
}
