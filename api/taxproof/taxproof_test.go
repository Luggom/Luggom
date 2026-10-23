package taxproof

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"
)

// htmlSandboxBR simula o comprovante atestado de um banco BR (PIX arrecadação
// IPTU) — mesmo formato do provider sandbox local (marco Viacredi, §13.58).
// Traz o E2E do PIX: é o payment_id canónico exigido pelo §13.146 B (a
// derivação por rótulos foi removida — um recibo sem campo canónico não mina).
const htmlSandboxBR = `<html><body>
<h1>Comprovante de Pagamento</h1>
<p class="data-tax-entity="br:sefaz_sp"">Prefeitura de São Paulo - SEFAZ</p>
<p>Favorecido: Prefeitura do Município de São Paulo</p>
<p>CNPJ: **392.130/0001-**</p>
<p>Pagador: JOAO DA SILVA</p>
<p>Valor: R$ 1.234,56</p>
<p>ID da operação: E99999991202609070000AR01BB02CC1</p>
<p>Autenticação: 3F2A9C11DD550E88</p>
<p>Tipo: ARRECADAÇÃO IPTU - BR.GOV.BR/AREC</p>
<p>{"data":"07/09/2026 10:11:12"}</p>
</body></html>`

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// httpResp monta uma resposta HTTP crua — que é o que o transcript atestado
// contém no `recv` (a API agora DERIVA status/corpo daqui, não do cliente).
func httpResp(body string) []byte {
	return []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: " +
		strconv.Itoa(len(body)) + "\r\n\r\n" + body)
}

func TestVerifyTlsnProofBR(t *testing.T) {
	// §13.145 (A10): o marcador do sandbox deixou de ser honrado — não há flag que
	// o ligue. O ente resolve-se pelo CNPJ do recebedor (é o que aqui se verifica).
	recv := httpResp(htmlSandboxBR)
	b := Bundle{
		SessionID:  "9f2c6d4e-1234-4a1b-8cde-0f1a2b3c4d5e",
		ServerName: "contaonline.viacredi.coop.br",
		Transcript: Transcript{Sent: b64([]byte("POST /servcorews/processJSON HTTP/1.1")), Recv: b64(recv)},
	}
	claims, anchor, err := VerifyTlsnProof("viacredi", b)
	if err != nil {
		t.Fatalf("VerifyTlsnProof: %v", err)
	}

	// §13.131 A3: a âncora é o CORPO revelado, não a mensagem HTTP inteira — os
	// cabeçalhos (Date, Set-Cookie…) mudam a cada prova e davam uma âncora NOVA
	// à MESMA página, pelo que o anti-replay nunca travava nada.
	wantAnchor := sha256.Sum256([]byte(htmlSandboxBR))
	if anchor != hex.EncodeToString(wantAnchor[:]) {
		t.Errorf("anchor mismatch: got %s want %s", anchor, hex.EncodeToString(wantAnchor[:]))
	}
	if claims.PaymentAmount != "1234.56" {
		t.Errorf("amount: got %q", claims.PaymentAmount)
	}
	if claims.PaymentCurrency != "BRL" {
		t.Errorf("currency: got %q", claims.PaymentCurrency)
	}
	if claims.TransactionID != "E99999991202609070000AR01BB02CC1" {
		t.Errorf("payment_id: got %q, quer o E2E canónico do rail (§13.146 B)", claims.TransactionID)
	}
	if !strings.Contains(claims.RecipientAccount, "392.130/0001") {
		t.Errorf("cnpj: got %q", claims.RecipientAccount)
	}
	if claims.PayerTaxID != "" {
		t.Errorf("payer tax id should be empty (individual payer), got %q", claims.PayerTaxID)
	}
	if claims.TaxEntityMatch != "br:iptu_sao_paulo" {
		// §13.145 (A10): o fixture traz um marcador `data-tax-entity` que dizia
		// "br:sefaz_sp". Honrá-lo era o furo — e estava até ERRADO: um IPTU é
		// municipal. Pelo CNPJ do recebedor (**.392.130/0001-**), o ente é o
		// municipal de São Paulo, que é o que se exige aqui.
		t.Errorf("tax entity: got %q", claims.TaxEntityMatch)
	}
	if claims.BankIdentity != "viacredi" {
		t.Errorf("bank identity: got %q", claims.BankIdentity)
	}
}

// §13.146 B — o vector que este teste tranca: o prover ESCONDE o campo canónico
// (o E2E do PIX) para forçar o fallback de rótulos e obter um payment_id
// DIFERENTE para o MESMO pagamento ⇒ o mesmo comprovante minava 2×. Aqui o
// recibo é o mesmo que `htmlSandboxBR` SEM a linha do E2E (a "redacção"
// diferente): tem de FALHAR FECHADO, não cair num rótulo genérico.
func TestVerifyTlsnProofBR_NoCanonicalID_FailsClosed(t *testing.T) {
	noE2E := `<html><body>
<h1>Comprovante de Pagamento</h1>
<p>Favorecido: Prefeitura do Município de São Paulo</p>
<p>CNPJ: **392.130/0001-**</p>
<p>Pagador: JOAO DA SILVA</p>
<p>Valor: R$ 1.234,56</p>
<p>ID da operação: ARREC20260907000123ABC</p>
<p>Autenticação: 3F2A9C11DD550E88</p>
<p>Tipo: ARRECADAÇÃO IPTU - BR.GOV.BR/AREC</p>
<p>{"data":"07/09/2026 10:11:12"}</p>
</body></html>`
	b := Bundle{
		SessionID:  "sem-e2e",
		ServerName: "contaonline.viacredi.coop.br",
		Transcript: Transcript{Recv: b64(httpResp(noE2E))},
	}
	_, _, err := VerifyTlsnProof("viacredi", b)
	if err == nil {
		t.Fatal("recibo SEM campo canónico (E2E) minou — o fallback de rótulos é escolhível pelo prover (§13.146 B)")
	}
	if !strings.Contains(err.Error(), "canonical payment id") {
		t.Fatalf("esperado erro de payment_id canónico, veio: %v", err)
	}
}

// §13.146 B — com DOIS E2E revelados o prover ESCOLHERIA qual usar (é o caso de
// um extracto com vários PIX), e o valor é lido independentemente do id: a
// escolha não seria uma identidade de pagamento. Fail-closed.
func TestVerifyTlsnProof_MultipleE2E_FailsClosed(t *testing.T) {
	body := `{"recebedorDocumento":"**.392.130/0001-**","beneficiario":"Prefeitura do Município","valor":"R$ 1.291,49","data":"15/07/2026 18:07:44","id1":"E99999991202601010000VC1YWGD3E47","id2":"E99999991202601010001VC1YWGD3E47","descricao":"ARRECADACAO IPTU"}`
	b := Bundle{
		SessionID:  "multi-e2e",
		ServerName: "internetbanking.caixa.gov.br",
		Transcript: Transcript{Recv: b64(httpResp(body))},
	}
	_, _, err := VerifyTlsnProof("caixa", b)
	if err == nil || !strings.Contains(err.Error(), "more than one distinct PIX E2E") {
		t.Fatalf("recibo com 2 E2E devia falhar fechado, veio: %v", err)
	}
}

// sampleViacrediPix é um recorte do comprovante REAL da Viacredi (PIX de IPTU para
// a Secretaria Municipal da Fazenda de SP, CNPJ mascarado "**.392.130/0001-**").
// Sem o marcador data-tax-entity: o ente tributário tem de ser resolvido PELO CNPJ.
const sampleViacrediPix = `{"data":{"OUTPUT_DATA":{"WS_RESULT":{"recebedorInstituicao":"BCO BRADESCO S.A.","recebedorNome":"SECRETARIA MUNICIPAL DA F","valorOriginalPix":"R$ 1.267,82","numeroDocumento":"867599291","recebedorDocumento":"**.392.130/0001-**","identificacao":"26073147005482490600399830","valor":"R$ 1.267,82","data":"15/07/2026 18:07:44","idTransacao":"E99999991202601010002VC1YWGD3E47"}},"RESPONSE_CODE":0}}`

func TestVerifyTlsnProofBR_RealViacrediPix(t *testing.T) {
	b := Bundle{
		SessionID:    "abc-1234",
		ServerName:   "contaonline.viacredi.coop.br",
		ResponseBody: sampleViacrediPix,
		Transcript:   Transcript{Recv: b64(httpResp(sampleViacrediPix))},
	}
	claims, _, err := VerifyTlsnProof("viacredi", b)
	if err != nil {
		t.Fatalf("VerifyTlsnProof: %v", err)
	}
	if claims.PaymentAmount != "1267.82" {
		t.Errorf("amount: got %q", claims.PaymentAmount)
	}
	if claims.TaxEntityMatch != "br:iptu_sao_paulo" {
		t.Errorf("tax entity: got %q (esperado br:iptu_sao_paulo via CNPJ mascarado)", claims.TaxEntityMatch)
	}
	if claims.TransactionID == "" {
		t.Errorf("transaction id vazio")
	}
}

// sampleViacrediPixFull é o comprovante REAL COMPLETO: multa/juros/descontos vêm
// ANTES do valor. Pegar o primeiro "R$" do corpo lia "multa":"R$ 0,00" (valor
// 0.00 → rejeição on-chain). O extrator tem de preferir o campo rotulado.
const sampleViacrediPixFull = `{"data":{"OUTPUT_DATA":{"WS_RESULT":{"recebedorInstituicao":"BCO BRADESCO S.A.","data":"15/07/2026 18:07:44","multa":"R$ 0,00","recebedorNome":"SECRETARIA MUNICIPAL DA F","valorOriginalPix":"R$ 1.291,49","protocolo":"2306.4030.1801.0F07.1A41.1A29.50","descontos":"R$ 0,00","valorTotal":"R$ 1.291,49","numeroDocumento":"867596174","juros":"R$ 0,00","valor":"R$ 1.291,49","valorTaxa":"R$ 0,00","idTransacao":"E99999991202601010001VC1YWGD3E47","recebedorDocumento":"**.392.130/0001-**"}},"RESPONSE_CODE":0}}`

func TestVerifyTlsnProofBR_RealViacrediPixFull(t *testing.T) {
	b := Bundle{
		SessionID:    "abc-full",
		ServerName:   "contaonline.viacredi.coop.br",
		ResponseBody: sampleViacrediPixFull,
		Transcript:   Transcript{Recv: b64(httpResp(sampleViacrediPixFull))},
	}
	claims, _, err := VerifyTlsnProof("viacredi", b)
	if err != nil {
		t.Fatalf("VerifyTlsnProof: %v", err)
	}
	if claims.PaymentAmount != "1291.49" {
		t.Errorf("amount: got %q (esperado 1291.49, não a multa 0,00)", claims.PaymentAmount)
	}
	if claims.TaxEntityMatch != "br:iptu_sao_paulo" {
		t.Errorf("tax entity: got %q", claims.TaxEntityMatch)
	}
	// A data tem de vir do comprovante (15/07/2026), não do relógio local —
	// senão a janela de 365 dias (max_age) nunca é aplicada de facto.
	if want := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC).Unix(); claims.PaymentDate != want {
		t.Errorf("payment date: got %d (%s), want %d (%s)",
			claims.PaymentDate, time.Unix(claims.PaymentDate, 0).UTC().Format("2006-01-02"),
			want, time.Unix(want, 0).UTC().Format("2006-01-02"))
	}
}

// Formato REAL do comprovante de PIX da Caixa (captura de 2026-09-17). Valores,
// ids e CNPJ são inventados — os FORMATOS são os do banco:
//   - o valor é string simples, SEM "R$" em lado nenhum (`"valor":"999,99"`);
//   - o E2E do PIX aparece em `id`;
//   - `conta.chave` é a chave PIX de um titular (UUID) — NÃO é id de pagamento.
const caixaPixReceipt = `{"detalheExtrato":{"E99999991202601010000VC1YWGD3E47":{"id":"E99999991202601010000VC1YWGD3E47","valor":"999,99","situacaoTransacao":"CONCLUIDA","autenticacao":"3F2A9C11DD550E88","usuarioPagador":{"cnpj":"","cpf":"00000000000","conta":{"agencia":null,"chave":"699dfe8a-4649-415f-8fe1-b91460d0fb87"}},"usuarioRecebedor":{"cnpj":"**.392.130/0001-**","cpf":"","conta":{"agencia":null,"chave":"22222222-3333-4444-5555-666666666666"}}}}}`

// caixaPixReceiptWithDate é o MESMO comprovante com a data do pagamento
// atestada — o rail tem de a trazer para a mineração ser preçada pela data.
const caixaPixReceiptWithDate = `{"detalheExtrato":{"E99999991202601010000VC1YWGD3E47":{"id":"E99999991202601010000VC1YWGD3E47","valor":"999,99","situacaoTransacao":"CONCLUIDA","data":"17/09/2026 10:11:12","autenticacao":"3F2A9C11DD550E88","usuarioPagador":{"cnpj":"","cpf":"00000000000","conta":{"agencia":null,"chave":"699dfe8a-4649-415f-8fe1-b91460d0fb87"}},"usuarioRecebedor":{"cnpj":"**.392.130/0001-**","cpf":"","conta":{"agencia":null,"chave":"22222222-3333-4444-5555-666666666666"}}}}}`

// O formato real capturado NÃO traz a data do pagamento. Sem data atestada não
// há preço na data do pagamento (o prover escolheria o dia) ⇒ fail-closed.
func TestVerifyTlsnProofCaixa_NoDate_FailsClosed(t *testing.T) {
	b := Bundle{
		SessionID:  "caixa-nodate",
		ServerName: "internetbanking.caixa.gov.br",
		Transcript: Transcript{Recv: b64(httpResp(caixaPixReceipt))},
	}
	_, _, err := VerifyTlsnProof("caixa", b)
	if err == nil || !strings.Contains(err.Error(), "no payment date") {
		t.Fatalf("recibo sem data devia falhar fechado, veio: %v", err)
	}
}

func TestVerifyTlsnProofCaixa_FormatoRealDePix(t *testing.T) {
	b := Bundle{
		SessionID:  "caixa-1",
		ServerName: "internetbanking.caixa.gov.br",
		Transcript: Transcript{Recv: b64(httpResp(caixaPixReceiptWithDate))},
	}
	claims, _, err := VerifyTlsnProof("caixa", b)
	if err != nil {
		t.Fatalf("VerifyTlsnProof: %v", err)
	}
	// O valor vem sem "R$" — tem de ser lido na mesma.
	if claims.PaymentAmount != "999.99" {
		t.Errorf("amount: got %q, quer 999.99", claims.PaymentAmount)
	}
	// O payment_id tem de ser o E2E do PIX, NUNCA a chave PIX do titular.
	if claims.TransactionID != "E99999991202601010000VC1YWGD3E47" {
		t.Errorf("payment_id: got %q, quer o E2E do PIX", claims.TransactionID)
	}
	if claims.TaxEntityMatch != "br:iptu_sao_paulo" {
		t.Errorf("tax entity: got %q", claims.TaxEntityMatch)
	}
	// O pagador ali é pessoa física (`cpf`, não `cnpj`) → sem ID de pagador, logo
	// o bloqueio 1206 não dispara.
	if claims.PayerTaxID != "" {
		t.Errorf("payer tax id: got %q, quer vazio (pagador pessoa física)", claims.PayerTaxID)
	}
}

func TestTaxEntityFromBody_ByNameFallback(t *testing.T) {
	// CRA: o slug da whitelist é "ca:cra" (antes devolvia "ca_cra" → rejeição on-chain).
	if got := taxEntityFromBody("<p>Canada Revenue Agency - GST/HST</p>", "Canada Revenue Agency", "CA"); got != "ca:cra" {
		t.Errorf("CRA: got %q, want ca:cra", got)
	}
	// EUA: condado sem EIN, resolvido pelo nome do BENEFICIÁRIO.
	if got := taxEntityFromBody("<p>Yavapai County Treasurer</p>", "Yavapai County Treasurer", "US"); got != "us:county:yavapai_az" {
		t.Errorf("US county: got %q", got)
	}
	// EUA: condado com o estado DEPOIS do nome ("County, ST") — desfaz a
	// ambiguidade de nomes repetidos (Orange existe em CA, FL e NY).
	if got := taxEntityFromBody("<p>Orange County, CA - Treasurer</p>", "Orange County, CA - Treasurer", "US"); got != "us:county:orange_ca" {
		t.Errorf("US county com estado: got %q, want us:county:orange_ca", got)
	}
	// Canadá: município pelo nome.
	if got := taxEntityFromBody("<p>City of Vancouver Property Tax</p>", "City of Vancouver", "CA"); got != "ca:city:vancouver" {
		t.Errorf("CA city: got %q", got)
	}
}

// A fraude que este teste tranca: o corpo atestado ESCOLHER o ente tributário.
// Qualquer página do banco pode conter o marcador (é só texto); honrá-lo deixava
// quem mina fixar o slug da whitelist sem resolução nenhuma.
//
// §13.145 (A10): o marcador deixou de ser honrado por COMPLETO — já não existe
// flag que o ligue, e o teste verifica isso mesmo com a variável de DEV definida.
func TestTaxEntityMarkerNeverHonored(t *testing.T) {
	body := `<p data-tax-entity="us:county:yavapai_az">Property Tax</p>`
	if got := taxEntityFromBody(body, "", "US"); got != "" {
		t.Errorf("marcador honrado (nunca deve ser): %q", got)
	}

	// A variável que o ligava em DEV já não é lida: definir não muda nada.
	t.Setenv("LUGGOM_TAX_ENTITY_MARKER", "1")
	if got := taxEntityFromBody(body, "", "US"); got != "" {
		t.Errorf("marcador honrado com a env de DEV definida: %q", got)
	}

	// Sem o marcador, o ente BR sai do CNPJ do recebedor (não do marcador).
	if got := taxEntityFromBody(htmlSandboxBR, "Prefeitura do Município de São Paulo", "BR"); got != "br:iptu_sao_paulo" {
		t.Errorf("BR devia resolver pelo CNPJ: %q", got)
	}
}

// A fraude que este teste tranca: o atacante abre QUALQUER página real do seu
// banco que apenas MENCIONE um ente da whitelist, sem pagar nada. A prosa não
// pode resolver o ente — só a linha do beneficiário resolve.
func TestTaxEntityFromBody_ProseAloneIsNotAPayee(t *testing.T) {
	if got := taxEntityFromBody("<p>Yavapai County Treasurer</p>", "", "US"); got != "" {
		t.Errorf("prosa sem beneficiário resolveu o ente: %q", got)
	}
	if got := taxEntityFromBody("<p>City of Vancouver Property Tax</p>", "", "CA"); got != "" {
		t.Errorf("prosa sem beneficiário resolveu o ente: %q", got)
	}
}

func TestTaxEntityFromBody_StateUsesRecipientNotBody(t *testing.T) {
	// O estado vem do nome do RECEBEDOR. Antes o fallback devolvia "ny_dtf"
	// fixo — qualquer imposto estadual dos EUA era creditado a Nova York.
	if got := taxEntityFromBody("<p>State tax payment</p>", "Texas Comptroller of Public Accounts", "US"); got != "us:tx" {
		t.Errorf("US state: got %q, want us:tx", got)
	}
	// Sem estado inequívoco no recebedor → fail-closed (não assume nenhum).
	if got := taxEntityFromBody("<p>State tax payment</p>", "", "US"); got != "" {
		t.Errorf("US state sem recebedor: got %q, want vazio", got)
	}
}

// (b) server_name tem de pertencer ao banco do provider.
func TestVerifyTlsnProofRejectsForeignHost(t *testing.T) {
	b := Bundle{
		SessionID:  "abc",
		ServerName: "evil.example.com",
		Transcript: Transcript{Recv: b64(httpResp(htmlSandboxBR))},
	}
	if _, _, err := VerifyTlsnProof("viacredi", b); err == nil {
		t.Fatal("esperado rejeitar server_name que não é do banco")
	}
}

// (a) O corpo vem do TRANSCRIPT — um response_body inventado pelo cliente é ignorado.
func TestVerifyTlsnProofIgnoresForgedBody(t *testing.T) {
	forged := `<p>CNPJ: 00.000.000/0001-00</p><p>Valor: R$ 9.999.999,99</p><p>ARRECADAÇÃO IPTU</p>`
	b := Bundle{
		SessionID:    "abc",
		ServerName:   "contaonline.viacredi.coop.br",
		Status:       200,
		ResponseBody: forged,
		Transcript:   Transcript{Recv: b64(httpResp(htmlSandboxBR))},
	}
	claims, _, err := VerifyTlsnProof("viacredi", b)
	if err != nil {
		t.Fatalf("VerifyTlsnProof: %v", err)
	}
	if claims.PaymentAmount == "9999999.99" {
		t.Fatalf("corpo FORJADO foi aceito (amount=%q)", claims.PaymentAmount)
	}
	if claims.PaymentAmount != "1234.56" {
		t.Errorf("amount: got %q (esperado 1234.56, do transcript)", claims.PaymentAmount)
	}
}

func TestParseHTTPResponseChunked(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n")
	st, body, err := parseHTTPResponse(raw)
	if err != nil || st != 200 || body != "hello world" {
		t.Fatalf("chunked: got (%d, %q, %v)", st, body, err)
	}
}

func TestVerifyTlsnProofRejectsNon2xx(t *testing.T) {
	b := Bundle{
		SessionID:  "abc",
		ServerName: "contaonline.viacredi.coop.br",
		Transcript: Transcript{Recv: b64([]byte("HTTP/1.1 500 Internal Server Error\r\nContent-Length: 4\r\n\r\noops"))},
	}
	if _, _, err := VerifyTlsnProof("viacredi", b); err == nil {
		t.Fatal("expected error for non-2xx attested status")
	}
}

func TestVerifyTlsnProofRejectsEmptyBody(t *testing.T) {
	b := Bundle{
		SessionID:  "abc",
		ServerName: "contaonline.viacredi.coop.br",
		Transcript: Transcript{Recv: b64(httpResp(""))},
	}
	if _, _, err := VerifyTlsnProof("viacredi", b); err == nil {
		t.Fatal("expected error for empty attested body")
	}
}

func TestVerifyTlsnProofRejectsBadBase64(t *testing.T) {
	b := Bundle{
		SessionID:  "abc",
		ServerName: "contaonline.viacredi.coop.br",
		Transcript: Transcript{Recv: "!!!not-base64!!!"},
	}
	if _, _, err := VerifyTlsnProof("viacredi", b); err == nil {
		t.Fatal("expected error for invalid transcript base64")
	}
}

func TestAnchorFallsBackToBody(t *testing.T) {
	b := Bundle{
		SessionID:    "abc",
		Status:       200,
		ResponseBody: "<html>comprovante</html>",
	}
	anchor, err := b.Anchor()
	if err != nil {
		t.Fatalf("Anchor: %v", err)
	}
	want := sha256.Sum256([]byte(b.ResponseBody))
	if anchor != hex.EncodeToString(want[:]) {
		t.Errorf("anchor mismatch: got %s want %s", anchor, hex.EncodeToString(want[:]))
	}
}

// §13.131 (2a): `amountDue` é o que se DEVE, não o que foi PAGO. Uma fatura ou
// um saldo de conta não pode minerar.
//
// Nota (§13.146 B): o corpo traz um E2E SINTÉTICO só para o portão canónico
// passar — sem ele, o recibo seria rejeitado antes deste guarda e o teste
// passaria pelo motivo ERRADO (deixava de exercitar o valor). O que se afere
// aqui é que `amountDue` NÃO é lido como valor pago.
func TestAmountDueIsNotPaidAmount(t *testing.T) {
	b := Bundle{
		SessionID:  "us-due",
		ServerName: "chase.com",
		Transcript: Transcript{Recv: b64(httpResp(
			`{"beneficiary":"Yavapai County Treasurer","amountDue":"$ 1,234.56","idTransacao":"E99999991202601010001VC1YWGD3E47","data":"15/01/2026 10:00:00","description":"property tax year 2026 parcel apn 12345"}`))},
	}
	_, _, err := VerifyTlsnProof("chase", b)
	if err == nil {
		t.Fatal("fatura (só amountDue) devia ser rejeitada, não minerada")
	}
	if !strings.Contains(err.Error(), "payment_amount") {
		t.Fatalf("esperado rejeição por valor em falta (amountDue não é valor pago), veio: %v", err)
	}
}

// §13.131 (2c): US/CA deixam de aceitar o "primeiro valor" da página — só um
// campo ROTULADO de valor pago conta (um `$` solto pode ser saldo/limite/taxa).
// E2E sintético pelo mesmo motivo do teste acima.
func TestUSFirstValueFallbackRemoved(t *testing.T) {
	b := Bundle{
		SessionID:  "us-free",
		ServerName: "chase.com",
		Transcript: Transcript{Recv: b64(httpResp(
			`{"beneficiary":"Yavapai County Treasurer","balance":"$ 9,999.99","idTransacao":"E99999991202601010001VC1YWGD3E47","data":"15/01/2026 10:00:00","description":"property tax year 2026 parcel apn 12345"}`))},
	}
	_, _, err := VerifyTlsnProof("chase", b)
	if err == nil {
		t.Fatal("valor solto ($) sem rótulo devia ser rejeitado (2c)")
	}
	if !strings.Contains(err.Error(), "payment_amount") {
		t.Fatalf("esperado rejeição por valor em falta, veio: %v", err)
	}
}

// §13.131 (2d) / §13.174: sem data ATESTADA, nenhum país mina — carimbar
// "agora" tornaria a janela de 365 dias (1205) inútil e, com o preço na data do
// pagamento, deixaria o prover escolher o preço. O BR deixou de ter fallback.
func TestPaymentDateFailClosed_AllCountries(t *testing.T) {
	casos := []struct {
		provider   string
		serverName string
		body       string
	}{
		{
			provider:   "chase",
			serverName: "chase.com",
			body:       `{"beneficiary":"Yavapai County Treasurer","amountPaid":"$ 240","description":"property tax year 2026 parcel apn 12345"}`,
		},
		{
			provider:   "viacredi",
			serverName: "contaonline.viacredi.coop.br",
			body:       `{"data":{"OUTPUT_DATA":{"WS_RESULT":{"recebedorNome":"SECRETARIA MUNICIPAL DA F","recebedorDocumento":"**.392.130/0001-**","valor":"R$ 1.267,82","idTransacao":"E99999991202601010002VC1YWGD3E47"}}}}`,
		},
	}
	for _, tc := range casos {
		b := Bundle{
			SessionID:  "nodate-" + tc.provider,
			ServerName: tc.serverName,
			Transcript: Transcript{Recv: b64(httpResp(tc.body))},
		}
		_, _, err := VerifyTlsnProof(tc.provider, b)
		if err == nil {
			t.Fatalf("%s sem data devia ser rejeitado (fail-closed)", tc.provider)
		}
		if !strings.Contains(err.Error(), "payment date") {
			t.Fatalf("%s: esperado erro de data, veio: %v", tc.provider, err)
		}
	}
}

// §13.131 (item 3): o ID do pagador vem SÓ do campo do pagador — e nunca o do
// recebedor (que é um ente da whitelist ⇒ dispararia o 1206 e bloquearia TUDO).
func TestPayerTaxID_FromPayerFieldOnly(t *testing.T) {
	if got := payerTaxID(`Pagador: ACME COMERCIO LTDA CNPJ 12.345.678/0001-90`, "BR"); got != "12345678000190" {
		t.Errorf("payer CNPJ: got %q, quer 12345678000190", got)
	}
	// Sem campo de pagador → vazio (o 1206 não dispara).
	if got := payerTaxID(`Recebedor CNPJ **.392.130/0001-**`, "BR"); got != "" {
		t.Errorf("sem pagador devia ser vazio, got %q", got)
	}
}

// A fraude que este teste tranca: o CNPJ do BENEFICIÁRIO a poucos caracteres do
// rótulo do pagador. Sem o corte no rótulo do beneficiário, seria lido como
// pagador — e o 1206 (governo→governo) rejeitaria toda a mineração legítima.
func TestPayerTaxID_StopsAtRecipientLabel(t *testing.T) {
	got := payerTaxID(`Pagador: JOAO DA SILVA Favorecido: **.392.130/0001-**`, "BR")
	if got != "" {
		t.Fatalf("CNPJ do favorecido lido como pagador: %q", got)
	}
}

// O pagador em JSON ANINHADO — o formato dos bancos BR — põe o CNPJ a ~60
// caracteres do rótulo, FORA da janela do nome. A janela do ID é maior
// (payerIDRe) e continua truncada no rótulo do beneficiário. Este teste pina a
// correcção, achada ao construir o formato BB do sandbox (cmd/sandbox-bank).
func TestPayerTaxID_NestedJSONPayerField(t *testing.T) {
	nested := `{"comprovante":{"beneficiario":{"documento":"**.392.130/0001-**"},"pagador":{"nome":"ACME COMERCIO LTDA","documento":"12.345.678/0001-90"}}}`
	if got := payerTaxID(nested, "BR"); got != "12345678000190" {
		t.Errorf("payer CNPJ (JSON aninhado): got %q, quer 12345678000190", got)
	}

	// E o ID do RECEBEDOR continua a não ser lido como pagador quando o pagador
	// só tem CPF (é o caso do comprovante real da Caixa).
	cpfPayer := `{"usuarioPagador":{"cnpj":"","cpf":"00000000000"},"usuarioRecebedor":{"cnpj":"**.392.130/0001-**"}}`
	if got := payerTaxID(cpfPayer, "BR"); got != "" {
		t.Errorf("pagador pessoa física devia ser vazio, got %q", got)
	}
}

const caixaPixReceiptPayerCNPJ = `{"detalheExtrato":{"E99999991202601010000VC1YWGD3E47":{"id":"E99999991202601010000VC1YWGD3E47","valor":"999,99","situacaoTransacao":"CONCLUIDA","data":"17/09/2026 10:11:12","usuarioPagador":{"cnpj":"12.345.678/0001-90","cpf":"","conta":{"agencia":null,"chave":"699dfe8a-4649-415f-8fe1-b91460d0fb87"}},"usuarioRecebedor":{"cnpj":"**.392.130/0001-**","cpf":"","conta":{"agencia":null,"chave":"22222222-3333-4444-5555-666666666666"}}}}}`

// O comprovante da Caixa traz `usuarioPagador` ANTES de `usuarioRecebedor`. Com o
// pagador a ser extraído (item 3), o CNPJ dele não pode ser confundido com o do
// recebedor — nem no `RecipientAccount` nem no `TaxEntityMatch`.
func TestVerifyTlsnProofCaixa_PayerIsNotTheRecipient(t *testing.T) {
	b := Bundle{
		SessionID:  "caixa-payer",
		ServerName: "internetbanking.caixa.gov.br",
		Transcript: Transcript{Recv: b64(httpResp(caixaPixReceiptPayerCNPJ))},
	}
	claims, _, err := VerifyTlsnProof("caixa", b)
	if err != nil {
		t.Fatalf("VerifyTlsnProof: %v", err)
	}
	if claims.PayerTaxID != "12345678000190" {
		t.Errorf("payer tax id: got %q, quer o CNPJ do PAGADOR", claims.PayerTaxID)
	}
	if !strings.Contains(claims.RecipientAccount, "392.130/0001") {
		t.Errorf("recipient account: got %q, quer o CNPJ do RECEBEDOR", claims.RecipientAccount)
	}
	if claims.TaxEntityMatch != "br:iptu_sao_paulo" {
		t.Errorf("tax entity: got %q (o pagador foi confundido com o recebedor?)", claims.TaxEntityMatch)
	}
}
