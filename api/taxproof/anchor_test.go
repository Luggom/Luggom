package taxproof

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// §13.131, achado A3 (decidido pelo Fundador em 2026-09-18): a âncora do
// anti-replay é o **CORPO revelado**, não a mensagem HTTP inteira.
//
// O bug: a âncora era `sha256(recv)` — cabeçalhos incluídos. Como `Date`,
// `Set-Cookie` e nonces mudam a cada pedido, re-provar a MESMA página produzia
// uma âncora NOVA, o `UsedDocumentHashes` nunca reconhecia a repetição, e o
// comprovante minava indefinidamente. Só o `payment_id` travava.
func TestAnchor_HeaderVolatilNaoMudaAAncora(t *testing.T) {
	body := `{"id":"pay-1","valor":"240.50","data":"2026-09-18"}`

	recv1 := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n" +
		"Date: Mon, 18 Sep 2026 10:00:00 GMT\r\nSet-Cookie: sess=AAA; Path=/\r\n\r\n" + body
	recv2 := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n" +
		"Date: Mon, 18 Sep 2026 10:00:07 GMT\r\nSet-Cookie: sess=BBB; Path=/\r\n\r\n" + body
	require.NotEqual(t, recv1, recv2, "as duas provas TÊM de diferir nos cabeçalhos — se não, o teste não prova nada")

	a1, err := Bundle{Transcript: Transcript{Recv: b64([]byte(recv1))}}.Anchor()
	require.NoError(t, err)
	a2, err := Bundle{Transcript: Transcript{Recv: b64([]byte(recv2))}}.Anchor()
	require.NoError(t, err)

	require.Equal(t, a1, a2,
		"a MESMA página tem de dar a MESMA âncora (era isto que falhava: a 2.ª mineração passava)")
}

// A âncora tem de ser igual nos dois caminhos: o da API (§13.121, que enche
// ResponseBody a partir do transcript) e o do notário (que traz o transcript).
func TestAnchor_MesmoValorNosDoisCaminhos(t *testing.T) {
	body := "corpo-do-comprovante"

	viaBody, err := Bundle{ResponseBody: body}.Anchor()
	require.NoError(t, err)

	recv := "HTTP/1.1 200 OK\r\nDate: Mon, 18 Sep 2026 10:00:00 GMT\r\n\r\n" + body
	viaTranscript, err := Bundle{Transcript: Transcript{Recv: b64([]byte(recv))}}.Anchor()
	require.NoError(t, err)

	require.Equal(t, viaBody, viaTranscript,
		"a API e o notário têm de calcular a MESMA âncora, senão a mineração legítima é recusada")
}

// Documentos diferentes continuam a dar âncoras diferentes (não se perdeu o
// anti-replay: perdeu-se só a dependência dos cabeçalhos).
func TestAnchor_DocumentosDiferentesDiferem(t *testing.T) {
	a, err := Bundle{ResponseBody: "comprovante-A"}.Anchor()
	require.NoError(t, err)
	b, err := Bundle{ResponseBody: "comprovante-B"}.Anchor()
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

// Sem separador de cabeçalhos (não é HTTP), cai no atestado inteiro — fail-safe.
func TestAnchor_NaoHTTPUsaOsBytesAtestados(t *testing.T) {
	a, err := Bundle{Transcript: Transcript{Recv: b64([]byte("isto-nao-e-uma-resposta-http"))}}.Anchor()
	require.NoError(t, err)
	require.NotEmpty(t, a)

	b, err := Bundle{Transcript: Transcript{Recv: b64([]byte("outra-coisa-qualquer"))}}.Anchor()
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

// Sem conteúdo nenhum, falha (não devolve âncora vazia = "nunca usado").
func TestAnchor_SemConteudoFalha(t *testing.T) {
	_, err := Bundle{}.Anchor()
	require.Error(t, err)

	_, err = Bundle{Transcript: Transcript{Recv: "!!!nao-e-base64!!!"}}.Anchor()
	require.Error(t, err)
}
