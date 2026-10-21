# Developer Guide — Testnet Luggom

Como a comunidade transforma um **banco / portal de impostos** em um **rail
minerável**. É a versão pública do que hoje vive em `docs/adapter-spec.md`.

> **Segurança:** esta pasta é pública. **Nunca** comite aqui chaves, seeds,
> keyrings, cookies de sessão ou recibos reais (PII). O recibo entra **mascarado**.

---

## 1. Quem faz o quê

| Parte | Quem faz | É código? |
|---|---|---|
| **Evidência** — recibo real sanitizado + campo canônico do rail | **comunidade** | não |
| **Sonda** — o rail responde a um cliente TLS independente? | **comunidade** | não (é uma *propriedade* da infra do banco) |
| **Ligar ao protocolo** — whitelist, host check, request builder, plugin | **núcleo/governança** | sim |

**Consequência prática:** um banco **não** precisa correr código nosso. Precisa
**servir o comprovante de forma proveitória** — e é isso que este guia valida.

---

## 2. Regra nº 1 — o anti-bot (a lição da Caixa)

O endpoint do comprovante **tem de responder a um cliente TLS independente** que
apresente o cookie de sessão — **não pode exigir token de dispositivo cunhado por
JavaScript e amarrado ao browser**. O TLSNotary termina o TLS **fora** do browser,
portanto imitar o browser não basta: tem de ser *o* browser.

Medição que fechou a Caixa (mesmo cookie, mesma sessão, headers exatos):

| Cliente | Resultado |
|---|---|
| Firefox **154** (browser real) | **200** |
| Firefox 147/148 (impersonation) | 302 |
| Chrome 150 (atual) | 302 |
| Safari 260 | 302 |

Um portal assim **não é adapter-ável**. **Antes de qualquer código, correr a sonda** (Passo 1).

---

## 3. Fluxo completo (resumo)

```
0. Evidência  →  1. Sonda  →  2. Código  →  3. Governança + release
```

---

## Passo 0 — Pacote de evidência (pré-requisito)

Isto é o que **quem minerou de verdade** entrega. Sem isto, ninguém escreve código.

1. **Capturar uma sessão real** — com a extensão (`coletor-browser/`), no banco
   alvo, faz o pagamento normalmente. A extensão grava pedidos/respostas + o
   cookie jar (inclusive `httpOnly`).
2. **Responder as 6 perguntas**:
   1. nickname do **beneficiário** (o ente tributário);
   2. o endpoint é **reflectivo**? (reproduz a consulta sem efeito colateral);
   3. o estado atestado é **"pago"**?;
   4. qual o **`payment_id` canónico** (único por pagamento)?
   5. o **valor pago** está **rotulado**? (formato exato);
   6. a **data** (com hora) está rotulada?
3. **Sanitizar o recibo** — mascarar PII (ex.: `**.392.130/0001-**`).
   **Regra dura: nunca commitar recibo real.**

### O campo canónico (o que serve e o que NÃO serve)

O `payment_id` tem de ser: **único por pagamento**, **um só** no comprovante e
**num campo rotulado** (não na prosa). Um `BN`/`EIN`/`APN`/`RT` é da **conta** e
repete-se entre pagamentos — bloquearia o 2.º pagamento como replay, ou daria um
id escolhível. É o mesmo erro da chave PIX (§13.133).

| Rail | Campo canónico (o que revelar) | NÃO serve |
|---|---|---|
| **BR — PIX** | **E2E do PIX** (32 chars: `E`+ISPB+AAAAMMDD+HHMM+11) | chave PIX (é do titular) |
| **EUA — IRS** (Direct Pay / EFTPS) | **confirmation number** | SSN/EIN, `tax year`, nº do formulário |
| **EUA — condado/estado** | **receipt/confirmation nº** do portal | APN/parcel, lote, `tax year` |
| **CA — CRA** | **confirmation number** | BN (9 dígitos), RT0001 |
| **CA — municipal** | **nº de recibo** | roll/avaliação, ano fiscal |

> **US/CA estão desligados de propósito** até existir este campo por rail. O código
> já está pronto a recebê-lo; falta a **evidência real**.

### Como se identifica o recebedor (BR → US/CA)

O Brasil é o modelo **testado e fechado**: no PIX de arrecadação o comprovante
traz o **CNPJ do recebedor** (a chave PIX é vinculada ao documento), então o
extrator lê o CNPJ e confirma que está na whitelist on-chain. É um identificador
forte — não há ambiguidade.

Nos EUA e no Canadá os recibos costumam **não** trazer o EIN/BN do fisco: o
"account number" que aparece é do próprio contribuinte (parcel/APN/roll/folio).
Por isso o recebedor é identificado em duas vias, da mais forte para a mais fraca.

> **Objetivo (spec): aproximar ao máximo o modelo BR.** No Brasil o recibo traz o
> **CNPJ do fisco** — identificador forte, testado e consolidado contra fraude.
> Ao trazer um recibo real US/CA, **escolher o portal/rail que exponha o EIN/BN do
> recebedor**. Quanto mais o rail se parecer com o BR (ID fiscal no comprovante),
> mais seguro e menos dependente dos guardas de nome. O fallback por nome é
> aceitável, mas é a via **mais fraca** — usá-lo só quando nenhum portal do rail
> mostrar o ID.

**1. ID fiscal, quando o recibo mostra (preferir sempre):**
- **EUA** — o **EIN** do recebedor (property tax / IRS).
- **Canadá — CRA (federal)** — **BN (9 dígitos) + RT0001**: aqui é **como o
  Brasil** — o comprovante traz o identificador do fisco, sem ambiguidade.
- **Canadá — municipal** — o **BN** do recebedor, quando aparece.

**2. Resolução por NOME, quando o ID não aparece (fallback):**
- **EUA** — property tax de condado: "Yavapai County Treasurer".
- **Canadá — municipal** — "City of Vancouver".
- O nome do **beneficiário** (nunca a prosa do corpo) resolve contra a whitelist.

**Guardas anti-fraude (já no código, obrigatórios para não inverter o modelo BR):**

- o nome/EIN só sai do campo do **beneficiário** (`Recipient`/`RecipientName`),
  nunca do texto do corpo — senão uma página que apenas *mencione* o fisco
  mineraria sem pagamento real;
- resolve **só se for inequívoco** — "Orange County" sem estado **não** resolve;
- **estado cruzado bloqueado** — "Suffolk County NY" não pode creditar a
  `suffolk_ma`;
- **concessionárias/serviços rejeitados** — "… Water District" não é imposto;
- exige **evidência fiscal específica** (property tax / parcel / APN / tax year /
  IRS) — não basta a palavra "county" ou "tax";
- o **EIN nunca é o do pagador** (só o do recebedor);
- **estados dos EUA não têm EIN único** → identificam-se **por nome**.

> **Regra de ouro para quem trouxer o recibo real:** usar o **ID fiscal (EIN/BN)
> sempre que o banco o mostrar**. O fallback por nome só existe porque muitos
> portais não mostram o ID — e, por isso mesmo, é o caminho que mais precisa dos
> guardas acima.

---

## Passo 1 — Sondar o rail

**Nesta ordem — a Caixa ensinou que inverter custa caro** (construiu-se o adapter
antes de saber se o rail passava; não passava).

```sh
python3 coletor-browser/verify_rail.py --banco <id>
```

| Resultado | Significado | Ação |
|---|---|---|
| **PASS** (200/400) | o rail responde a um cliente independente | avançar para o Passo 2 |
| **FAIL** (302 `sessaoEncerradaNovo`) | o WAF exige o próprio browser | **PARAR** — não construir adapter |
| outro | ler o corpo | repetir com captura fresca antes de decidir |

> Se a captura tiver **mais de 5 min**, um FAIL pode ser só sessão caída — repete
> com uma fresca antes de desistir.

---

## Passo 2 — Checklist de código (9 pontos + 1 teste)

| # | Ficheiro | O quê |
|---|---|---|
| 1 | `api/reclaimverify/providers.go` (`KnownProviders`) | `{ID, Name, Country, Domain}`. **Sem `Domain`, o host check é fail-closed** |
| 2 | `x/luggom/types/operational_lists.go` (`DefaultBankProviders`) | o ID na whitelist **on-chain** (gov 67%). **Sem isto, `/mine/tax` dá 403/1208** |
| 3 | `api/taxnature/taxnature.go` (`Country`) | o ID no `switch` do país. **Sem isto, `VerifyTlsnProof` aborta** |
| 4 | `coletor-browser/make_prover_request_<banco>.py` | o **request builder** (molde: **Caixa** se cookie-only; **Viacredi** se assinatura ECDSA/RSA) |
| 5 | `coletor-browser/manifest.json` | o domínio em `host_permissions` **e** `content_scripts.matches` |
| 6 | `tlsn-verifier/packages/plugins/src/luggom_tax_<banco>.plugin.ts` + `registry.ts` | o plugin do minerador desktop (hoje cada banco é um plugin) |
| 7 | `api/taxproof/taxproof.go` | **só se** o comprovante não casar as heurísticas genéricas de BR — aqui entra o **regex canónico por rail** (US/CA) |
| 8 | `api/taxproof/taxproof_test.go` | teste de formato (molde: Viacredi/Caixa) |
| 9 | `cmd/sandbox-bank` | o formato no sandbox, para desenvolver **sem conta real** |

**Extra:** `api/taxproof/providers_sync_test.go` obriga (1) e (2) a não divergir.

---

## Passo 3 — Governança + release

- **Whitelist on-chain:** o slug entra por `MsgAddBankProvider` (**governança**),
  não pelo dev individual. Na coorte da testnet, entra via `DefaultBankProviders`.
- **Re-assinar o `.xpi`** (o `manifest.json` mudou) e **re-lançar o app**
  (`.deb`/`.AppImage` + `SHA256SUMS`).
- **Recompilar a API** (mudou `providers.go`/`taxproof.go`/`taxnature.go`).

> **Trabalho futuro (item de testnet):** tornar o domínio **dado on-chain** em vez
> de código (`bank_providers` guarda o domínio; a API lê on-chain; o coletor usa
> `host_permissions` amplo + `scripting.executeScript` dinâmico). Isto elimina o
> **re-assinar** a cada banco novo — mas **não** elimina a evidência nem o extrator.

---

## 5. Limites técnicos

| Limite | Valor | Porquê |
|---|---|---|
| Tamanho do **pedido** | ≤ **4096 bytes** | teto do prover TLSNotary (`maxSentData`) |
| **Compressão** da resposta | **não** | o extractor lê o corpo em claro |
| Endpoint | **same-origin**, JSON preferido | é o que o host check valida |
| Resposta | **2xx** | outro código nunca é comprovante |

## 6. O que um banco NÃO resolve

- **Débito automático / convênio NÃO é minerável.** O comprovante atesta o
  **intermédio** (a arrecadação do banco), não o **ente** (a Fazenda). O imposto
  tem de ser **pago manualmente** (guia/boleto, ou PIX-arrecadação com o QR da guia).
- **Pagar em conta que não a do titular** não ajuda: o recibo tem de ser da sessão
  do próprio utilizador.

## 7. Privacidade (as duas fases)

- **Produção:** *selective disclosure* — revela-se **só** os campos do comprovante
  (incl. o id canónico); o resto fica **comprometido**, não revelado.
- **Desenvolvimento:** a captura **contém PII**. Antes de virar fixture/spec, tem
  de ser **mascarada**. **Nunca** publicar recibo real.

## 8. Como validar (antes de fechar)

```sh
export PATH="$PWD/.go1.23.8/bin:$PATH"

# API + chain
go test ./api/taxproof/... ./api/taxnature/... ./api/reclaimverify/... ./x/luggom/...

# desktop (prover + carteira)
cargo test -p luggom-core -p luggom-wallet && cargo clippy --all-targets

# sonda do rail novo
python3 coletor-browser/verify_rail.py --banco <id>
```

## 9. Estado por banco (hoje)

| Banco | Whitelist | Request builder | Plugin | Rail | E2E real |
|---|---|---|---|---|---|
| **Viacredi/Ailos** | ✅ | ✅ | ✅ | ✅ | ✅ **minerado `code 0`** |
| **Caixa** | ✅ | ✅ | ✅ | ❌ **FAIL (anti-bot)** | ❌ |
| **BB** | ✅ | 🟡 sandbox | 🟡 | pendente | ❌ |
| Sandbox (`localhost:9000`) | — | ✅ | ✅ | ✅ | ✅ |
| **US/CA** (chase, rbc, …) | ✅ on-chain | — | — | — | ❌ falta campo canónico |

