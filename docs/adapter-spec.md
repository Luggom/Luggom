# Spec do Adapter de Banco — Luggom

> **ARTEFACTO DE PUBLICAÇÃO INTENCIONAL.** É o documento que um banco/portal lê
> para se tornar **minerável**. Não está no git (`docs/` está no `.gitignore`) —
> vive no **tarball**, como `api/` e `scripts/`. Publicar deliberadamente, como o
> `TESTNET/`.
>
> **Versão:** 2026-09-18 · **Estado:** derivado do que está **provado** (Viacredi/Ailos,
> mineração real `code 0`) e das **duas lições que custaram sessões** na Caixa
> (§13.127 e §13.137-B). Inclui o **§4-B**: o que falta para os rails **US/CA** —
> o **id canónico do pagamento** —, que é o que a coorte da testnet tem de trazer.

---

## 0. O que é (e o que não é)

O Luggom minera LUG pagando **imposto real**, provado por **TLSNotary**: o utilizador
prova, sem terceiros e sem KYC, que uma transacção fiscal existe, e o protocolo
extrai dela um conjunto fixo de *claims*.

**O adapter é o contrato entre o banco/portal e o protocolo.** Não é um SDK, não é
uma integração: é a garantia de que o **comprovante** que o banco serve contém o que
o protocolo precisa, e de que ele é **alcançável por um cliente TLS independente**.

---

## 1. As duas partes do contrato

| Parte | Quem faz | É código? |
|---|---|---|
| **Rail** — o comprovante responde a um cliente TLS que **não** é o browser, com o cookie de sessão | o banco/portal | não — é uma **propriedade** da infra |
| **Claims** — o comprovante traz os campos obrigatórios, extraíveis | o banco/portal | não — é **formato** |
| Ligar as duas ao protocolo | **Luggom** | sim (whitelist, host check, request builder, plugin) |

**Consequência prática:** nada disto exige que o banco corra código nosso. Exige que
ele **sirva o comprovante de uma forma proveitória**.

---

## 2. ⚠️ Requisito anti-bot (a lição da Caixa — §13.137-B)

**O endpoint do comprovante tem de responder a um cliente TLS independente que
apresente o cookie de sessão** — não pode exigir um *token de dispositivo cunhado
por JavaScript* e amarrado ao browser.

Medição que fechou a Caixa (mesma cookie, mesma sessão, headers exactos do browser):

| Cliente | Resultado |
|---|---|
| Firefox **154** (o browser real) | **200** |
| Firefox 147/148 (o mais novo que existe em impersonation) | 302 |
| **Chrome 150** (browser atual) | 302 |
| Safari 260 | 302 |

Ou seja: **não basta imitar um browser** — tem de ser *o* browser. Um portal assim
**não é adapter-ável** por este protocolo, porque o TLSNotary termina sempre o TLS
**fora** do stack do browser.

> **Critério de aceitação nº 1.** Antes de qualquer trabalho de adapter:
> **correr a sonda** — §6.

## 2-B. ⚠️ Dois requisitos que a app desktop revelou (2026-09-24, §13.166)

**1) Localização.** O banco **recusa o acesso** sem acesso à localização ("é necessário
que o acesso à localização esteja habilitado"). O cliente tem de a fornecer — num
webview do sistema isso não vem de graça.

**2) O anti-bot pode desafiar o *cliente*, não o TLS.** Testado com o DataDome na
Viacredi: o webview do sistema (WebKitGTK) — **mesmo sem qualquer instrumentação
nossa** — recebe um **captcha que não renderiza**, e o login pára. O **Firefox real**
passa. Consequência para o contrato: um rail pode ser **inalcançável dentro do app**
mesmo com o TLS a passar; o cliente **tem de ser o browser** que o banco aceita.
*(Mesma lição da Caixa, §13.137, por outra via.)*

---

## 3. Modelo de autenticação aceite

Dois modelos são aceites, **desde que reprodutíveis fora do browser**:

1. **REST/JSON same-origin autenticado por cookie** *(preferido)* — é o mais simples:
   o cookie de sessão é capturado pelo jar (inclui `httpOnly`) e o pedido é
   reconstruído tal e qual.
2. **Assinatura por pedido** — aceite **se as chaves forem capturáveis na sessão**.
   A Viacredi/Ailos assina com **ECDSA P-256** (`Access-Signature`) + **RSA-OAEP**
   (`X-Request`) e **está provada ponta-a-ponta**: as chaves efémeras são apanhadas
   na própria sessão e o pedido é reassinado.

**Não aceite:** esquemas cujo segredo seja **inacessível ao cliente** (ex.: chave
dentro de um enclave/WebCrypto cujo material nunca sai, ou um token de dispositivo
cunhado por JS com binding ao browser — ver §2).

---

## 4. Formato do comprovante — campos obrigatórios

| # | Campo | Exemplos reais | O que resolve |
|---|---|---|---|
| 1 | **Valor PAGO, rotulado** | `valorTotal`, `valorPago`, `amountPaid` | o montante minerado. **Nunca** `amountDue`/`valorAPagar` (§13.135): isso é o que se **deve** |
| 2 | **Data** (com hora) | `"data":"15/07/2026 18:07:44"` | a janela de **365 dias** (`ErrStalePayment`, 1205) |
| 3 | **ID do ENTE como BENEFICIÁRIO** | `recebedorDocumento`, `usuarioRecebedor.cnpj` | o *slug* do ente tributário. **Nunca** da prosa do documento, **nunca** do pagador |
| 4 | **ID único do pagamento** | **E2E do PIX** (32 chars: `E`+ISPB+AAAAMMDD+HHMM+11) em `id`/`idTransacao` | o anti-replay on-chain (`ErrPaymentIDAlreadyUsed`, 1202) |
| 5 | **Pista de natureza fiscal** | `ARRECADAÇÃO IPTU`, `DARF`, `GPS`, `property tax` | o portão anti-ciclo (`ErrMissingTaxNature`, 1207) |
| 6 | **ID do pagador** (se empresa) | `usuarioPagador.cnpj` | o portão governo↔governo (1206) |

**Notas de extracção que o banco deve conhecer:**
- O valor pode vir **sem símbolo de moeda** (`"valor":"999,99"`) — o extrator lê-o
  na mesma (segunda passagem), mas o **rótulo tem de ser de valor pago**.
- O ID único deve ser o **E2E/end-to-end** quando existir: é o único identificador
  **único por pagamento** e independente do banco. Chaves PIX **não** servem — são
  do titular e repetem-se entre pagamentos.
- O **pagador** é lido do seu próprio campo (a janela trunca no rótulo do
  beneficiário), precisamente para o ID do recebedor nunca ser confundido com ele.

---

## 4-B. ⚠️ US/Canadá — o campo que FALTA (id canónico do pagamento)

> **Estado: NÃO minerável — de propósito.** Desde o §13.146/§13.147 o `payment_id`
> tem de ser o **campo canónico do rail** (`api/taxproof/taxproof.go`): sem ele a
> extracção **falha fechado**. No Brasil esse campo é o **E2E do PIX**; nos EUA e no
> Canadá **ainda não está definido**, logo estes rails estão desligados. Para os
> ligar **não basta código**: faltam **recibos reais** que revelem o campo.

### Porquê (é a regra, não uma preferência)
Com *selective disclosure* o prover **escolhe** o que revela. Se o comprovante tiver
mais de um id plausível, quem mina escolhe o que lhe dá um `payment_id` **novo** e o
**mesmo pagamento mina duas vezes**. Por isso o id tem de ser:

1. **único por PAGAMENTO** (não por conta, titular ou bem);
2. **um só** no comprovante (com mais de um id distinto o protocolo recusa);
3. **num campo rotulado** — não na prosa do documento.

### O que serve e o que NÃO serve
| Rail | Campo canónico (o que a coorte deve fazer revelar) | NÃO serve (repete-se entre pagamentos) |
|---|---|---|
| **EUA — IRS** (Direct Pay / EFTPS) | **número de confirmação** do pagamento (`confirmation number`) | SSN/EIN, `tax year`, nº do formulário (1040) |
| **EUA — condado/estado** | **nº de recibo/confirmação** do portal (varia: `confirmation`, `receipt #`, `payment reference`) | **APN/parcel**, nº do lote, `tax year` |
| **CA — CRA** | **número de confirmação** do pagamento (`confirmation number`) | **BN** (9 dígitos) e **RT0001** — são a **conta**, repetem-se; NEQ (Québec) não é aceite |
| **CA — municipal** | **nº de recibo** do pagamento | nº da `roll`/avaliação, ano fiscal |

⚠️ Um **BN/EIN/APN/RT** como `payment_id` bloquearia o **2.º pagamento** do mesmo
contribuinte como *replay* (falso positivo) — ou, pior, daria um id escolhível. É o
mesmo erro da chave PIX (§13.133), que é do **titular**, não do pagamento.

### O que a coorte tem de entregar (por portal)
Um **pacote de evidência**, **sanitizado** (ver "Privacidade" abaixo):

1. **Recibo real** onde o campo canónico apareça **rotulado** (captura da resposta
   JSON/HTML do portal).
2. Resposta às **6 perguntas** do §13.131 (nickname do beneficiário · endpoint
   **reflectivo**? · estado **"pago"** · **`payment_id` canónico** · valor rotulado · data).
3. O **formato exacto** do **valor pago** (rotulado) e da **data** (com hora).
4. O **`server_name`** (domínio) — é o que o host check valida.
5. Resultado da **sonda de rail** (`verify_rail.py` → §6). **Sem PASS, não há adapter.**

O campo é depois **codificado por rail** em `api/taxproof/taxproof.go` (um regex
canónico por rail) e **pinado por um fixture** que passa pelo **extractor real** —
exactamente como o sandbox faz para o BR. Até lá o código **já está pronto a
receber o campo**; o que falta é a evidência.

### Privacidade — as duas fases
- **Em produção:** a privacidade é a *selective disclosure* — o utilizador revela
  **só** os campos do comprovante (incl. o id canónico); o resto fica comprometido,
  **não** revelado. Nada disto exige o corpo inteiro (era a opção que **não** se tomou).
- **Em desenvolvimento:** o artefacto capturado **contém PII**. Antes de virar
  fixture/spec tem de ser **mascarado** (como as fixtures actuais:
  `**.392.130/0001-**`). Regra dura: **nunca** se publica um recibo real — a captura
  da Caixa, por exemplo, está marcada "contém PII — NÃO commitar".

### Critério de aceitação (quando se liga um rail US/CA)
- [ ] um campo canónico **único por pagamento**, definido e codificado (um regex por rail);
- [ ] um **fixture sanitizado** que o extractor real lê (teste verde);
- [ ] o **fail-closed mantido**: sem o campo → **não mina**; com mais de um id distinto → **não mina**;
- [ ] `provider_id` na whitelist **on-chain** + ente na whitelist (já existem para US/CA);
- [ ] sonda de rail **PASS** (§6) para cada domínio novo.

---

## 5. Limites técnicos

| Limite | Valor | Porquê |
|---|---|---|
| Tamanho do **pedido** | ≤ **4096 bytes** | tecto do prover TLSNotary (o `maxSentData`); pedidos maiores falham |
| **Compressão** da resposta | **não** | o extractor lê o corpo atestado em claro; `Content-Encoding` esconde-o |
| Endpoint | **same-origin**, JSON preferido | é o que o host check valida (`server_name` = domínio do banco) |
| Resposta | **2xx** | qualquer outro código nunca é comprovante |

---

## 6. Como se VALIDA um banco novo (nesta ordem)

> **Esta ordem é a correcção que a Caixa impôs.** Construiu-se o adapter antes de
> saber se o rail passava; não passava.

1. **Capturar** — com a extensão (`coletor-browser/`) no banco, o utilizador faz o
   que faria normalmente; a extensão grava pedidos/respostas + o cookie jar.
2. **Sondar o rail** — `python3 coletor-browser/verify_rail.py --banco <id>`:
   - **PASS** (200/400) ⇒ o rail responde a um cliente independente. Avançar.
   - **FAIL** (302 de fachada) ⇒ **PARAR.** Não construir o adapter.
   - ⚠️ Se a captura tiver mais de 5 min, um FAIL pode ser só a sessão caída —
     repetir com uma fresca antes de decidir.
3. **Só então** construir o adapter — §7.

---

## 7. Checklist — adicionar um banco

| # | Onde | O que |
|---|---|---|
| 1 | `api/reclaimverify/providers.go` (`KnownProviders`) | `{ID, Name, Country, Domain}`. **Sem `Domain` o host check é fail-closed** |
| 2 | `x/luggom/types/operational_lists.go` (`DefaultBankProviders`) | o ID na whitelist **on-chain** (gov 67%). **Sem isto, `/mine/tax` devolve 403/1208** |
| 3 | `api/taxnature/taxnature.go` (`Country`) | o ID no `switch` do país. **Sem isto, `VerifyTlsnProof` aborta** |
| 4 | `coletor-browser/make_prover_request_<banco>.py` | o **request builder** (molde: o da Caixa se for cookie-only; o da Viacredi se tiver assinatura) |
| 5 | `coletor-browser/manifest.json` | o domínio em `host_permissions` **e** `content_scripts.matches` |
| 6 | `tlsn-verifier/packages/plugins/src/luggom_tax_<banco>.plugin.ts` + `registry.ts` | o plugin do minerador desktop (hoje cada banco é um plugin — o `luggom_tax.plugin.ts` está hardcoded à Caixa) |
| 7 | `api/taxproof/taxproof.go` | **só se** o comprovante não casar as heurísticas genéricas de BR (rótulos/regex) |
| 8 | `api/taxproof/taxproof_test.go` | teste do formato do comprovante (molde: os casos Viacredi/Caixa) |
| 9 | `cmd/sandbox-bank` | o formato no sandbox, para se desenvolver **sem conta real** |

**Extra:** as duas listas (1) e (2) estão protegidas por um teste de deriva
(`api/taxproof/providers_sync_test.go`) — se divergirem, a suíte falha.

---

## 8. O que o banco NÃO resolve (regras de negócio)

- **Débito automático / convênio NÃO é minerável** (§13.127, decisão fechada). O
  comprovante atesta o **intermédio** (a arrecadação do banco), não o **ente**
  (a Fazenda). O código **não** tem fallback por nome para BR — é deliberado.
  ⇒ Para minerar, o imposto tem de ser **pago manualmente** com o CNPJ do ente
  (guia/boleto, ou PIX-arrecadação com o QR da guia).
- **Pagar o imposto numa conta que não a do titular** não ajuda: o recibo tem de
  ser da sessão do próprio utilizador.

---

## 9. Estado por banco (2026-09-18)

| Banco | Whitelist | Request builder | Plugin | Testes | Rail | E2E real |
|---|---|---|---|---|---|---|
| **Viacredi/Ailos** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ **minerado `code 0`** |
| **Caixa** | ✅ | ✅ | ✅ | ✅ | ❌ **FAIL (§13.137-B)** | ❌ |
| **BB** | ✅ | 🟡 contra o sandbox | 🟡 | 🟡 | **pendente de sessão** | ❌ |
| Sandbox (`localhost:9000`) | — | ✅ | ✅ | ✅ | ✅ | ✅ |

*Os bancos dos EUA/Canadá (chase, rbc, …) estão na whitelist **on-chain** mas **não são mineráveis**: falta o **id canónico do pagamento** e a validação com recibo real (§13.131 e **§4-B**). Não usar esta tabela como cobertura — ver o §4-B para o que a coorte tem de trazer.*
