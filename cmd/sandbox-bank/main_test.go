package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"luggom/api/taxproof"
)

// newClient devolve um cliente com jar que já fez o "login" (tem o cookie de
// sessão) — é o que as rotas de comprovante exigem.
func newClient(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	c := &http.Client{Jar: jar}
	resp, err := c.Get(srv.URL + "/")
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return c
}

// do executa o pedido da rota e devolve o corpo.
func do(t *testing.T, c *http.Client, base string, f formato) (int, string, http.Header) {
	t.Helper()
	req, err := http.NewRequest(f.metodo, base+f.rota, nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body), resp.Header
}

// Sem cookie de sessão, TODAS as rotas de comprovante redirecionam para o login.
func TestRotas_ExigemSessao(t *testing.T) {
	srv := httptest.NewServer(newMux())
	defer srv.Close()
	c := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	for _, f := range formatos {
		req, err := http.NewRequest(f.metodo, srv.URL+f.rota, nil)
		require.NoError(t, err)
		resp, err := c.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusFound, resp.StatusCode, "sem cookie: %s", f.rota)
	}
}

// Com sessão, cada rota serve o seu formato e assina-o com X-Receipt-SHA256.
func TestRotas_ServemComSessaoEAssinam(t *testing.T) {
	srv := httptest.NewServer(newMux())
	defer srv.Close()
	c := newClient(t, srv)

	for _, f := range formatos {
		status, body, hdr := do(t, c, srv.URL, f)
		require.Equal(t, http.StatusOK, status, f.rota)
		require.Equal(t, f.corpo, body, f.rota)

		sum := sha256.Sum256([]byte(f.corpo))
		require.Equal(t, hex.EncodeToString(sum[:]), hdr.Get("X-Receipt-SHA256"), f.rota)
	}
}

// httpResp embrulha o corpo numa resposta HTTP crua (é o que o transcript
// atestado contém no `recv`).
func httpResp(body string) []byte {
	return []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: " +
		strconv.Itoa(len(body)) + "\r\n\r\n" + body)
}

// O teste que dá sentido ao sandbox: cada formato servido tem de ser lido pelo
// EXTRATOR REAL (o mesmo que o notário corre). Se um formato deixar de ser
// extraído, o adapter daquele banco parte — e é aqui que se sabe.
func TestFormatos_SaoExtraidosPeloExtratorReal(t *testing.T) {
	// §13.145 (A10): já não há escape por env. O sandbox serve em localhost, mas
	// o `server_name` que aqui se constrói é o DOMÍNIO REAL do banco de cada
	// formato — que é o que a prova atestaria. O host check corre a sério.
	srv := httptest.NewServer(newMux())
	defer srv.Close()
	c := newClient(t, srv)

	casos := []struct {
		rota       string
		host       string
		provider   string
		amount     string
		entity     string
		txID       string
		payerTaxID string
	}{
		{
			rota: "/servcorews/processJSON", host: "contaonline.viacredi.coop.br", provider: "viacredi",
			amount: "1291.49", entity: "br:iptu_sao_paulo",
			txID: "E99999991202601010000VC1YWGD3E47", payerTaxID: "",
		},
		{
			rota: "/sinbc-pix/nb/extrato/comprovantePixExtratoSucesso", host: "internetbanking.caixa.gov.br", provider: "caixa",
			amount: "1291.49", entity: "br:iptu_sao_paulo",
			txID: "E99999991202601010000VC1YWGD3E47", payerTaxID: "",
		},
		{
			// O formato BB é uma REFERÊNCIA nossa (não há recibo real): exercita o
			// pagador EMPRESA (CNPJ) — o caso que alimenta o portão 1206.
			rota: "/bb/nb/comprovantes/consultar", host: "www.bb.com.br", provider: "banco_do_brasil",
			amount: "1291.49", entity: "br:iptu_sao_paulo",
			txID: "E99999991202601010000VC1YWGD3E47", payerTaxID: "12345678000190",
		},
	}

	for _, tc := range casos {
		t.Run(tc.rota, func(t *testing.T) {
			var f formato
			for _, x := range formatos {
				if x.rota == tc.rota {
					f = x
				}
			}
			require.NotEmpty(t, f.rota, "rota não encontrada em formatos")

			_, body, _ := do(t, c, srv.URL, f)
			b := taxproof.Bundle{
				SessionID:  "sandbox",
				ServerName: tc.host,
				Transcript: taxproof.Transcript{Recv: base64.StdEncoding.EncodeToString(httpResp(body))},
			}
			claims, _, err := taxproof.VerifyTlsnProof(tc.provider, b)
			require.NoError(t, err)
			require.Equal(t, tc.amount, claims.PaymentAmount, "valor")
			require.Equal(t, tc.entity, claims.TaxEntityMatch, "ente")
			require.Equal(t, tc.txID, claims.TransactionID, "payment_id")
			require.Equal(t, tc.payerTaxID, claims.PayerTaxID, "payer_tax_id")
		})
	}
}

// §13.146 B — o `/comprovante` (formato HTML genérico) NÃO traz o campo canónico
// (E2E do PIX), pelo que deixa de ser extraível: um recibo sem campo canónico não
// mina. Este teste pina essa consequência — sem ele, a perda de cobertura ficava
// silenciosa (o caso não está na lista `casos` acima, que só cobre os 3 rails com
// E2E). Se se definir um campo canónico para este rail, este teste passa a ser o
// sítio onde se prova a cobertura nova.
func TestComprovanteSemCampoCanonico_FalhaFechado(t *testing.T) {
	srv := httptest.NewServer(newMux())
	defer srv.Close()
	c := newClient(t, srv)

	var f formato
	for _, x := range formatos {
		if x.rota == "/comprovante" {
			f = x
		}
	}
	require.NotEmpty(t, f.rota, "formato /comprovante não encontrado")

	_, body, _ := do(t, c, srv.URL, f)
	b := taxproof.Bundle{
		SessionID:  "sandbox",
		ServerName: "contaonline.viacredi.coop.br",
		Transcript: taxproof.Transcript{Recv: base64.StdEncoding.EncodeToString(httpResp(body))},
	}
	_, _, err := taxproof.VerifyTlsnProof("viacredi", b)
	require.Error(t, err, "o /comprovante (sem E2E) TEM de falhar fechado (§13.146 B)")
	require.Contains(t, err.Error(), "canonical payment id")
}
