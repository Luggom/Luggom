// Package taxproof adapta a API `/mine/tax` para a proof TLSNotary REAL
// (passo 2 do marco §13.58). Enquanto o fluxo legado (mock) aceita um JSON
// de prova declarado pelo cliente, aqui as claims são extraídas do CONTEÚDO
// ATESTADO — o corpo da resposta revelado pela prova (selective disclosure) —
// e o anti-replay é ancorado no hash do transcript criptográfico (sent/recv).
//
// Limitação honesta (Fase 1, trust API): a autenticidade criptográfica final é
// garantida pelo notário TLSNotary; nesta camada garantimos consistência e
// rastreabilidade — a proof não pode afirmar dados que não estejam no corpo
// atestado que a própria proof revela. A verificação criptográfica on-chain
// (fases 2/3) e o corpo notarizado saem daqui sem mudança de contrato.
package taxproof

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"luggom/api/reclaimverify"
	"luggom/api/taxnature"
	govtypes "luggom/x/luggom/types"
)

// Transcript é o transcript criptográfico da prova, em base64 (como enviado
// pelo prover UniFFI no APK).
type Transcript struct {
	Sent string `json:"sent"` // base64
	Recv string `json:"recv"` // base64
}

// Bundle é o payload de prova real enviado pelo APK após `proveFinalize`.
type Bundle struct {
	SessionID    string     `json:"session_id"`
	ServerName   string     `json:"server_name"`
	Status       int        `json:"status"`
	ResponseBody string     `json:"response_body"` // corpo revelado (HTML/texto do comprovante atestado)
	Transcript   Transcript `json:"transcript"`
}

// Anchor returns the hex SHA-256 anchoring the proof for anti-replay.
//
// ⚠️ A âncora é o **CORPO revelado**, NÃO a mensagem HTTP inteira (§13.131 A3,
// decidido pelo Fundador em 2026-09-18). O hash da mensagem inteira incluía os
// CABEÇALHOS — e `Date`, `Set-Cookie` e nonces mudam a cada pedido — logo
// re-provar a MESMA página dava sempre uma âncora NOVA: o `UsedDocumentHashes`
// (que guarda este valor) nunca travava nada, e o comprovante minava as vezes
// que se quisesse. Só o `payment_id` segurava.
//
// O corpo revelado é estável para o mesmo documento SÓ SE a redacção for a
// mesma: com selective disclosure o prover escolhe o que revela, pelo que duas
// provas do mesmo documento com redacções diferentes dão âncoras DIFERENTES
// (falsificado no review adversarial de 2026-09-18 — o comentário anterior dizia
// "a MESMA âncora" e estava errado). Por isso a âncora é o **2.º travão**: o
// 1.º é o `payment_id` canónico (§13.146 B), que não depende da redacção.
func (b Bundle) Anchor() (string, error) {
	body := []byte(b.ResponseBody)

	if len(body) == 0 && b.Transcript.Recv != "" {
		raw, err := base64.StdEncoding.DecodeString(b.Transcript.Recv)
		if err != nil {
			return "", fmt.Errorf("invalid recv transcript base64: %w", err)
		}
		// Do transcript atestado, interessa o CORPO (os cabeçalhos fora).
		if _, parsed, err := parseHTTPResponse(raw); err == nil {
			body = []byte(parsed)
		} else {
			// Não é HTTP (ou não foi possível partir): usa-se o atestado inteiro.
			body = raw
		}
	}

	if len(body) == 0 {
		return "", fmt.Errorf("no attested content (recv transcript or response_body)")
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// ---------------------------------------------------------------------------
// Extração de claims do corpo atestado (selective disclosure server-side)
// ---------------------------------------------------------------------------

// regexes genéricos por país (heurística; afinado por banco na FASE 8/marco real).
var (
	// Moeda/valor. Só o BR mantém o fallback "primeiro valor" (há bancos BR
	// ainda não testados); US/CA exigem um campo ROTULADO de valor pago (§13.131, 2c).
	brAmountRe = regexp.MustCompile(`(?i)(?:R\$\s?|BRL\s?)[\s]*([0-9][0-9.,]*)`)

	// Identificadores
	brCnpjRe = regexp.MustCompile(`[0-9*]{2,}(\.[0-9*]{3})?\.?[0-9*]{3}/[0-9*]{4}-[0-9*]{2,}`)
	usEinRe  = regexp.MustCompile(`\b\d{2}-\d{7}\b`)
	caBnRe   = regexp.MustCompile(`\b\d{9}\b`)
	// (O regex `txidRe` de rótulos genéricos — nº do documento, autenticação,
	// operação… — foi REMOVIDO no §13.146 B. Ver o uso do payment_id: a
	// derivação por rótulos era o caminho que o prover podia ESCOLHER.)
	//
	// pixE2ERe: o E2E do PIX — E + ISPB(8) + AAAAMMDD(8) + HHMM(4) + 11
	// alfanuméricos (32 caracteres). É o id CANÓNICO do pagamento: único por
	// pagamento, estável, independente do banco/rail e NÃO escolhível pelo
	// prover. Confirmado em recibos reais da Caixa e da Viacredi. Uma chave PIX
	// não serve (é do TITULAR, estável entre pagamentos).
	pixE2ERe = regexp.MustCompile(`\bE[0-9]{20}[A-Za-z0-9]{11}\b`)

	// Natureza fiscal (anti-cycle §13.34)
	brNatureRe = regexp.MustCompile(`(?i)(arrecada[cç][aã]o|BR\.GOV\.BR|DARF|GPS|IPTU|IPVA|ISS|ICMS|ITR|taxa)`)
	usNatureRe = regexp.MustCompile(`(?i)(property tax|parcel|apn\b|tax year|\bIRS\b|federal tax|1040|estimated tax)`)
	caNatureRe = regexp.MustCompile(`(?i)(CRA|BN.{0,4}RT|property tax|municipal tax|tax bill|GST|HST)`)

	// Nome do beneficiário (para slug + payer)
	payerNameRe = regexp.MustCompile(`(?i)(?:pagador|payer|cliente|conta de origem)[^<\n]{0,60}`)
	recipNameRe = regexp.MustCompile(`(?i)(?:favorecido|benefici[aá]rio|recipient|beneficiary)[^<\n]{0,60}`)
	// payerIDRe é a janela usada SÓ para procurar o ID do pagador: JSONs
	// aninhados (ex.: `"pagador":{"documento":"12.345.678/0001-90"}`) põem o
	// CNPJ a ~60 chars do rótulo, fora da janela do nome. Continua truncada no
	// rótulo do beneficiário (payerFieldStopRe), que é o que impede ler o ID do
	// RECEBEDOR como pagador.
	payerIDRe = regexp.MustCompile(`(?i)(?:pagador|payer|cliente|conta de origem)[^<\n]{0,160}`)
	// payerFieldStopRe corta a janela do PAGADOR no rótulo do BENEFICIÁRIO
	// seguinte. Sem isto, o CNPJ do recebedor (por definição um ente da
	// whitelist) poderia ser lido como pagador e o bloqueio 1206
	// (governo→governo) rejeitaria TODA a mineração (§13.131, item 3).
	payerFieldStopRe = regexp.MustCompile(`(?i)favorecido|benefici[aá]rio|recipient|beneficiary|recebedor`)

	// Data do pagamento no corpo atestado (comprovante Ailos/Viacredi:
	// "data":"15/07/2026 18:07:44"). Exige o formato com hora para não casar com
	// outros campos "data" genéricos (dataReferencia, dataLancamento, ...).
	receiptDateRe = regexp.MustCompile(`(?i)"data"\s*:\s*"(\d{2}/\d{2}/\d{4})\s+\d{2}:\d{2}:\d{2}"`)
)

func stripHTML(s string) string {
	re := regexp.MustCompile(`(?s)<[^>]*>`)
	s = re.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	// colapsa espaços
	sp := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(sp.ReplaceAllString(s, " "))
}

func normMoney(s string) string {
	s = strings.ReplaceAll(s, ".", "")
	return strings.ReplaceAll(s, ",", ".")
}

// moneyPrefixByCountry: prefixo de moeda usado nas regexes de valor.
var moneyPrefixByCountry = map[string]string{
	"BR": `R\$\s?|BRL\s?`,
	"US": `US\$\s?|\$\s?`,
	"CA": `C\$\s?|CAD\s?`,
}

// amountLabelsByCountry: campos rotulados que representam o VALOR PAGO.
// Comprovantes reais (ex.: Viacredi/Ailos) trazem multa/juros/descontos ANTES
// do valor total — pegar o primeiro "R$" do corpo lia "multa":"R$ 0,00".
// A ordem vai do mais específico ao mais genérico.
//
// `amountDue`/`valorAPagar` NÃO constam (§13.131, 2a/2b): são o que se DEVE, não
// o que foi PAGO — uma fatura ou um saldo de conta passaria a minerar.
var amountLabelsByCountry = map[string][]string{
	"BR": {"valorTotal", "valorPago", "valorLiquido", "valorOriginalPix", "valorOriginal", "valorDocumento", "valor", "total"},
	"US": {"amountPaid", "totalAmount", "paymentAmount", "amount", "total", "value"},
	"CA": {"amountPaid", "totalAmount", "paymentAmount", "amount", "total", "value"},
}

// labeledAmount devolve o valor associado a um rótulo conhecido (ex.
// "valorTotal":"R$ 1.291,49" ou "Valor: R$ 1.234,56"), ou "" se nenhum casar.
//
// Duas passagens, por causa de bancos que NÃO põem o símbolo da moeda:
//  1. rótulo + prefixo de moeda ("R$ 1.291,49") — o caso da Viacredi/Ailos;
//  2. rótulo + string JSON puramente numérica ("valor":"999,99") — o caso da
//     Caixa, cujo comprovante de PIX não tem "R$" em lado nenhum do corpo.
//
// A ordem dos rótulos mantém-se em ambas, para o valor TOTAL continuar a ganhar
// quando existe (é o que evita ler a "multa":"R$ 0,00").
func labeledAmount(text, country string) string {
	prefix, ok := moneyPrefixByCountry[country]
	if !ok {
		return ""
	}
	for _, lab := range amountLabelsByCountry[country] {
		re := regexp.MustCompile(`(?i)"?` + regexp.QuoteMeta(lab) + `"?\s*[:=]\s*"?\s*(?:` + prefix + `)[\s]*([0-9][0-9.,]*)`)
		if m := re.FindStringSubmatch(text); len(m) > 1 {
			return m[1]
		}
	}
	// Sem prefixo de moeda: exige aspas E string puramente numérica, para não
	// capturar `"total":3` nem números soltos que não sejam valores.
	for _, lab := range amountLabelsByCountry[country] {
		re := regexp.MustCompile(`(?i)"` + regexp.QuoteMeta(lab) + `"\s*:\s*"([0-9][0-9.,]*)"`)
		if m := re.FindStringSubmatch(text); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// digitsOnly keeps only the digits of a tax ID (complete or masked), matching
// the whitelist normalization (ex.: "**.392.130/0001-**" → "3921300001").
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// (a) Parsing do transcript atestado: status + corpo do HTTP REALMENTE provado
// ---------------------------------------------------------------------------

func parseAttestedResponse(recvB64 string) (int, string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(recvB64))
	if err != nil {
		return 0, "", fmt.Errorf("invalid recv transcript base64: %w", err)
	}
	return parseHTTPResponse(raw)
}

func parseHTTPResponse(raw []byte) (int, string, error) {
	sep := []byte("\r\n\r\n")
	i := bytes.Index(raw, sep)
	if i < 0 {
		sep = []byte("\n\n")
		i = bytes.Index(raw, sep)
	}
	if i < 0 {
		return 0, "", fmt.Errorf("no header/body separator in attested response")
	}
	head := strings.ReplaceAll(string(raw[:i]), "\r\n", "\n")
	body := raw[i+len(sep):]

	lines := strings.Split(head, "\n")
	if len(lines) == 0 {
		return 0, "", fmt.Errorf("empty attested response")
	}
	parts := strings.Fields(lines[0])
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "HTTP/") {
		return 0, "", fmt.Errorf("bad attested status line %q", lines[0])
	}
	status, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, "", fmt.Errorf("bad attested status code %q", parts[1])
	}

	chunked := false
	for _, l := range lines[1:] {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(k), "Transfer-Encoding") &&
			strings.Contains(strings.ToLower(v), "chunked") {
			chunked = true
		}
	}
	if chunked {
		body, err = decodeChunked(body)
		if err != nil {
			return 0, "", err
		}
	}
	return status, string(body), nil
}

func decodeChunked(b []byte) ([]byte, error) {
	var out []byte
	for {
		line, rest := cutLine(b)
		if line == nil {
			return nil, fmt.Errorf("chunked: truncated chunk size")
		}
		b = rest
		sizeStr := strings.TrimSpace(string(line))
		if i := strings.IndexByte(sizeStr, ';'); i >= 0 {
			sizeStr = sizeStr[:i]
		}
		n, err := strconv.ParseInt(sizeStr, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("chunked: bad chunk size %q", sizeStr)
		}
		if n == 0 {
			return out, nil
		}
		if int64(len(b)) < n {
			return nil, fmt.Errorf("chunked: truncated chunk data")
		}
		out = append(out, b[:n]...)
		b = b[n:]
		_, b = cutLine(b) // consome o CRLF após o chunk
	}
}

// cutLine devolve a linha (sem CRLF) e o restante; line == nil se não houver linha.
func cutLine(b []byte) ([]byte, []byte) {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		line := b[:i]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		return line, b[i+1:]
	}
	return nil, nil
}

// taxEntityFromBody determina o slug do ente tributário a partir do corpo
// atestado, por scanning de nome/CNPJ/EIN/BN conhecido. A whitelist on-chain
// valida o slug final no keeper.
//
// recipientLine é o nome do recebedor extraído do comprovante. Serve apenas
// para resolver o ESTADO dos EUA: as abreviaturas estaduais ("in", "or", "me")
// casariam palavras comuns se fossem procuradas no corpo inteiro.
func taxEntityFromBody(body string, recipientLine string, country string) string {
	// O corpo atestado NUNCA fixa o slug do ente (§13.145, A10): qualquer página
	// do banco pode conter um marcador `data-tax-entity="…"` (é só texto) e, se o
	// honrasse, quem mina escolheria o ente da whitelist sem resolução nenhuma.
	// O ente sai do CNPJ/EIN/BN do RECEBEDOR.
	low := strings.ToLower(stripHTML(body))
	switch country {
	case "BR":
		// 1) Pelo CNPJ do recebedor (completo ou mascarado) → ente tributário da
		// whitelist. Rail-agnóstico (PIX/boleto/DARF). É o caso dos comprovantes
		// reais, ex.: "**.392.130/0001-**" → br:iptu_sao_paulo.
		// O campo do PAGADOR é mascarado: se o CNPJ dele vier primeiro (Caixa), o
		// "primeiro CNPJ do corpo" leria o pagador e não o ente (§13.131, residual (e)).
		if m := brCnpjRe.FindString(maskPayerField(body)); m != "" {
			if slug, ok := govtypes.WhitelistedEntityForID(digitsOnly(m)); ok {
				return slug
			}
		}
		// 2) Por nome (Receita Federal / STN / SEFAZ)
		switch {
		case strings.Contains(low, "receita federal"):
			return "br:receita_federal"
		case strings.Contains(low, "tesouro"):
			return "br:stn"
		case strings.Contains(low, "sefaz"):
			return "br:sefaz_sp" // fallback genérico SEFAZ; lugar-geográfico afinado por banco
		}
	case "US":
		// O ente tem de ser o BENEFICIÁRIO do pagamento, não uma menção em
		// prosa no corpo: a prosa é escolhida pelo atacante (qualquer página do
		// seu próprio banco pode citar um condado da whitelist sem que exista
		// pagamento algum), enquanto a linha do beneficiário é o campo do
		// próprio pagamento.
		rlow := strings.ToLower(recipientLine)
		switch {
		case strings.Contains(rlow, "irs") || strings.Contains(rlow, "internal revenue") || strings.Contains(rlow, "federal tax") || strings.Contains(rlow, "1040"):
			return "irs"
		case strings.Contains(rlow, "franchise tax board") || strings.Contains(rlow, "california"):
			return "ca_ftb"
		}
		// Fisco ESTADUAL: um departamento de receita estadual não tem EIN
		// próprio — identifica-se por nome. Sem estado inequívoco não se assume
		// nenhum (fail-closed: a tx é rejeitada por tax_entity inválido). Antes
		// devolvia "ny_dtf" fixo — qualquer estado era creditado a Nova York.
		if taxnature.IsUSStateRevenueDept(recipientLine) {
			if slug := taxnature.USStateSlug(recipientLine); slug != "" {
				return slug
			}
		}
		// Comprovantes dos EUA normalmente NÃO trazem o EIN do fisco (só o nome:
		// "Yavapai County Treasurer"). Resolve pelo nome — só quando inequívoco.
		if slug, ok := govtypes.ResolveTaxEntityByName(recipientLine); ok {
			return slug
		}
	case "CA":
		rlow := strings.ToLower(recipientLine)
		if strings.Contains(rlow, "cra") || strings.Contains(rlow, "canada revenue agency") || containsBnRt(rlow) {
			return "ca:cra"
		}
		// Idem Canadá: município pelo nome ("City of Vancouver") quando o
		// comprovante não traz o BN do ente.
		if slug, ok := govtypes.ResolveTaxEntityByName(recipientLine); ok {
			return slug
		}
	}
	return ""
}

func containsBnRt(s string) bool {
	return regexp.MustCompile(`\b\d{9}\s*rt\d{4}\b|\brt\d{4}\b`).MatchString(s)
}

// VerifyTlsnProof valida a estrutura da proof real e devolve as claims
// extraídas do corpo atestado + o anchor de anti-replay.
func VerifyTlsnProof(providerID string, b Bundle) (*reclaimverify.VerifiedClaims, string, error) {
	if strings.TrimSpace(b.SessionID) == "" {
		return nil, "", fmt.Errorf("tlsn_proof.session_id is empty")
	}

	// (b) O `server_name` atestado TEM de pertencer ao banco do provider — senão
	// um atacante prova contra um servidor próprio e declara o banco. Fail-closed.
	if err := reclaimverify.ProviderHostAllowed(providerID, b.ServerName); err != nil {
		return nil, "", fmt.Errorf("tlsn proof host check: %w", err)
	}

	// (a) Status e corpo são DERIVADOS do transcript atestado — não confiamos no
	// que o cliente declara em `status`/`response_body` (senão daria pra juntar um
	// transcript real com um comprovante inventado e minerar sem pagar).
	if strings.TrimSpace(b.Transcript.Recv) == "" {
		return nil, "", fmt.Errorf("tlsn_proof.transcript.recv is required (attested response)")
	}
	status, body, err := parseAttestedResponse(b.Transcript.Recv)
	if err != nil {
		return nil, "", fmt.Errorf("attested transcript is not a valid HTTP response: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, "", fmt.Errorf("tlsn proof status must be 2xx (got %d)", status)
	}
	if strings.TrimSpace(body) == "" {
		return nil, "", fmt.Errorf("tlsn_proof attested body is empty (nothing was attested)")
	}
	b.Status = status
	b.ResponseBody = body

	anchor, err := b.Anchor()
	if err != nil {
		return nil, "", err
	}

	country := taxnature.Country(providerID)
	if country == "" {
		return nil, "", fmt.Errorf("unknown provider country for provider_id %q", providerID)
	}

	text := stripHTML(b.ResponseBody)
	claims := &reclaimverify.VerifiedClaims{
		BankIdentity: providerID,
	}

	switch country {
	case "BR":
		if v := labeledAmount(text, "BR"); v != "" {
			claims.PaymentAmount = normMoney(v)
			claims.PaymentCurrency = "BRL"
		} else if m := brAmountRe.FindStringSubmatch(text); len(m) > 1 {
			claims.PaymentAmount = normMoney(m[1])
			claims.PaymentCurrency = "BRL"
		}
		// CNPJ do BENEFICIÁRIO (mantém máscara/barra p/ taxnature). O campo do
		// PAGADOR é mascarado antes do scan: se o CNPJ do pagador vier primeiro
		// (é o caso da Caixa: `usuarioPagador` antes de `usuarioRecebedor`), o
		// "primeiro CNPJ do corpo" (§13.131, residual do fix (e)) leria o pagador
		// e resolveria o ente tributário errado.
		if m := brCnpjRe.FindString(maskPayerField(text)); m != "" {
			claims.RecipientAccount = m
		}
	case "US":
		// Sem fallback "primeiro valor" (2c): `$` casa saldo/limite/taxa em
		// qualquer ponto da página — só um campo ROTULADO de valor pago conta.
		if v := labeledAmount(text, "US"); v != "" {
			claims.PaymentAmount = normMoney(v)
			claims.PaymentCurrency = "USD"
		}
		if m := usEinRe.FindString(maskPayerField(text)); m != "" {
			claims.RecipientAccount = m
		}
	case "CA":
		// Idem US (2c): sem fallback "primeiro valor".
		if v := labeledAmount(text, "CA"); v != "" {
			claims.PaymentAmount = normMoney(v)
			claims.PaymentCurrency = "CAD"
		}
		if m := caBnRe.FindString(maskPayerField(text)); m != "" {
			claims.RecipientAccount = m
		}
	}

	// payment_id: SÓ o campo canónico do rail. A derivação por RÓTULOS foi
	// REMOVIDA (§13.146 B): o prover escolhe o que revela (selective
	// disclosure), e ESCONDER o E2E fazia-o cair no fallback de rótulos ⇒ um
	// payment_id DIFERENTE para o MESMO pagamento ⇒ o comprovante minava 2×.
	//
	// MAIS DE UM E2E DISTINTO também é fail-closed: com dois ids revelados o
	// prover ESCOLHERIA qual usar (um extracto com vários PIX), e o valor é lido
	// independentemente do id — a escolha não seria uma identidade de pagamento.
	// (O mesmo E2E repetido — a chave de `detalheExtrato` e o `id` do comprovante
	// da Caixa — conta UMA vez: é o mesmo pagamento.)
	// (O valor continua a não estar ligado ao id por construção: residual
	// registado; o rótulo do valor é do comprovante, não da linha do E2E.)
	e2eIDs := pixE2ERe.FindAllString(text, -1)
	distinct := make(map[string]struct{}, len(e2eIDs))
	for _, id := range e2eIDs {
		distinct[id] = struct{}{}
	}
	if len(distinct) > 1 {
		return nil, "", fmt.Errorf("attested receipt has more than one distinct PIX E2E: the prover cannot be allowed to choose the payment id (§13.146 B)")
	}
	if len(e2eIDs) > 0 {
		claims.TransactionID = e2eIDs[0]
	}
	if p := recipientName(text); p != "" {
		claims.RecipientName = p
	}
	if p := payerName(text); p != "" {
		claims.PayerAccount = p
	}
	// Item 3 (§13.131): o ID do pagador, extraído SÓ do campo do pagador. Alimenta
	// o bloqueio 1206 (governo→governo) — antes ficava sempre vazio e o portão
	// estava morto. Vai no digest assinado (AttestationFields.PayerTaxID).
	claims.PayerTaxID = payerTaxID(text, country)
	claims.TaxEntityMatch = taxEntityFromBody(b.ResponseBody, claims.RecipientName, country)
	claims.Description = natureFragment(text, country)
	// Data do pagamento: vem do CORPO ATESTADO (o comprovante traz a data da
	// transação). Sem data atestada, FAIL-CLOSED para TODOS os países: carimbar
	// "agora" deixaria o prover escolher o dia — e, com o preço na data do
	// pagamento, escolher o preço. (Antes só US/CA falhavam; BR caía no relógio.)
	if claims.PaymentDate == 0 {
		claims.PaymentDate = receiptPaymentDate(text)
	}
	if claims.PaymentDate == 0 {
		return nil, "", fmt.Errorf("attested receipt has no payment date: a prover-selectable date is refused")
	}
	if claims.PaymentDate > time.Now().Unix()+300 {
		return nil, "", fmt.Errorf("attested receipt has a payment date in the future")
	}

	// §13.146 B — o anti-replay é SEMÂNTICO (a identidade do pagamento), não os
	// bytes: a âncora (A3) oscila com a redacção, e só o campo canónico do rail
	// é estável e não-escolhível pelo prover. Sem ele, FAIL-CLOSED: não se
	// minera com um id que o prover pode escolher. (Um rail sem campo canónico
	// definido NÃO minera — é o preço de fechar o vector; ver a pendência da
	// "cadeia de alias para recibos sem E2E", §13.132 §8.)
	if claims.TransactionID == "" {
		return nil, "", fmt.Errorf("attested receipt has no canonical payment id for its rail: a prover-selectable id is refused (§13.146 B)")
	}

	if err := reclaimverify.ValidateClaims(claims); err != nil {
		return nil, "", fmt.Errorf("attested receipt failed validation: %w", err)
	}
	return claims, anchor, nil
}

func recipientName(text string) string {
	if m := recipNameRe.FindString(text); m != "" {
		return strings.TrimSpace(regexp.MustCompile(`(?i)(favorecido|benefici[aá]rio|recipient|beneficiary)[:\s]*`).ReplaceAllString(m, ""))
	}
	return ""
}

// receiptPaymentDate extrai a data do pagamento do corpo atestado.
// Devolve 0 quando o comprovante não traz a data explícita (o chamador decide o
// fallback). A janela de tax_payment_max_age_days (365d) só é aplicada de facto
// se a data vier da prova — carimbar "agora" tornaria o limite inútil.
func receiptPaymentDate(text string) int64 {
	m := receiptDateRe.FindStringSubmatch(text)
	if len(m) < 2 {
		return 0
	}
	t, err := time.ParseInLocation("02/01/2006", m[1], time.UTC)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func payerName(text string) string {
	if m := payerNameRe.FindString(text); m != "" {
		return strings.TrimSpace(regexp.MustCompile(`(?i)(pagador|payer|cliente|conta de origem)[:\s]*`).ReplaceAllString(m, ""))
	}
	return ""
}

// payerFieldSpan devolve o intervalo [start,end) do CAMPO do PAGADOR: a janela
// após o rótulo do pagador (payerIDRe — mais larga, para alcançar o ID em JSON
// aninhado), truncada no rótulo do beneficiário seguinte (payerFieldStopRe).
// ok=false quando o documento não tem rótulo de pagador.
func payerFieldSpan(text string) (int, int, bool) {
	loc := payerIDRe.FindStringIndex(text)
	if loc == nil {
		return 0, 0, false
	}
	end := loc[1]
	if stop := payerFieldStopRe.FindStringIndex(text[loc[0]:loc[1]]); stop != nil {
		end = loc[0] + stop[0]
	}
	return loc[0], end, true
}

// payerTaxID extrai o CNPJ/EIN/BN do PAGADOR do respectivo campo.
//
// Porquê o corte: o ID do RECEBEDOR é, por definição, um ente da whitelist. Se a
// janela do pagador o apanhasse, o bloqueio 1206 (governo→governo) rejeitaria
// TODA a mineração (§13.131, item 3). Sem ID no campo do pagador devolve "" — o
// 1206 simplesmente não dispara.
func payerTaxID(text, country string) string {
	start, end, ok := payerFieldSpan(text)
	if !ok {
		return ""
	}
	m := text[start:end]
	switch country {
	case "BR":
		if c := brCnpjRe.FindString(m); c != "" {
			return digitsOnly(c)
		}
	case "US":
		if e := usEinRe.FindString(m); e != "" {
			return e
		}
	case "CA":
		if b := caBnRe.FindString(m); b != "" {
			return b
		}
	}
	return ""
}

// maskPayerField substitui o CAMPO do pagador por espaços, preservando o
// comprimento. Evita que o scan do RECEBEDOR apanhe o ID do pagador quando este
// aparece primeiro no corpo (§13.131, residual do fix (e)).
func maskPayerField(text string) string {
	start, end, ok := payerFieldSpan(text)
	if !ok {
		return text
	}
	return text[:start] + strings.Repeat(" ", end-start) + text[end:]
}

func natureFragment(text string, country string) string {
	switch country {
	case "BR":
		return brNatureRe.FindString(text)
	case "US":
		return usNatureRe.FindString(text)
	case "CA":
		return caNatureRe.FindString(text)
	}
	return ""
}
