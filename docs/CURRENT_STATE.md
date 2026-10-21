# CURRENT_STATE — registo de gates de qualidade

> **O que é:** o registo que o papel **DV Master** exige (`.cursor/skills/luggom-dv-master`) ao concluir um **gate de qualidade** — data, papel, achados (crítico/médio/baixo) e commit.
>
> **Nota de armazenamento:** `docs/` está no `.gitignore` ⇒ vive no **tarball**, como o `api/` e os `scripts/`.

---

## 2026-09-18 — Gate: **Fecho da FASE 7** (pré-hard test)

- **Papel:** DV Master (verificação de código: correcção, edge cases, erros engolidos, anti-replay/whitelist, cobertura de testes, integração).
- **Commit:** `52b1304` (A+B fechados) + `c5cf5f2` (spec US/CA §4-B), sobre a série do dia (`7de5898`, `ebc2eae`, `45c9966`, `6466deb`, `6305013` → `c0fe35d`, `e1c3573`).
- **Suíte:** `go test ./x/luggom/... ./api/... ./cmd/luggom-api/... ./app/... ./cmd/sandbox-bank/...` **verde**; os 4 binários reconstruídos (`build/{luggomd,luggom-api,luggom-extract,sandbox-bank}`) e o `luggom-extract` **provado a falhar fechado** sem E2E.
- **Fecho:** ✅ **GATE FECHADO — 2026-09-18.** As **duas assinaturas** foram dadas (`docs/CHECKLIST_FECHO_FASE7.md` §5): **Fundador** (fichas A–D aceites, itens 1/4/6/7 ok, **D6 confirmado**) e **DV Master** (parecer APROVADO). A **FASE 7 está FECHADA**; a **FASE 8** abre pelo **Bloco 5 (desktop)** — roteiro em `docs/PLANO_DESKTOP_E_HARDTEST.md`.
- **Parecer:** ✅ **APROVADO — os 2 CRÍTICOS estão FECHADOS** (§13.147). O parecer esteve **RETIDO** durante o dia de 2026-09-18 (§13.146): dois agentes independentes provaram, com reprodução, que (1) `MsgMine`/`MsgMineUSD` cunhavam **com zero prova** — 1.000.000 LUG reproduzidos — e (2) a âncora do anti-replay era **escolhida pelo prover**, permitindo minar o mesmo comprovante duas vezes. O Fundador decidiu **A = desactivar as msgs** e **B = exigir o campo canónico do rail, com fail-closed**; a implementação (§13.147) foi verificada por um **2.º review adversarial** (2 agentes), que ainda apanhou e fechou **binários `build/` stale** (o que o notário usa!), o CLI `mine-tax-payment` sem `provider_id` e um comentário do filtro estrito que sobrestimava a protecção. **O gate ficou FECHADO** com as duas assinaturas (ver o "Fecho" acima).

### Achados

| # | Gravidade | Achado | Evidência | Estado |
|---|---|---|---|---|
| M1 | **Médio** | **4 envios com erro ENGOLIDO** em `distributeMiningSplit`: **Fundador**, **Staking Reservoir**, **ATOM Reservoir** e **POOL treasury** — se um falhar, o LUG fica no módulo e a tx reporta **sucesso** | `x/luggom/keeper/otc_vault.go` (era `_ = err` em 4 sítios) | ✅ **CORRIGIDO** — todos fail-closed, e o `founder_address` inválido também falha (antes era saltado em silêncio); 4 testes novos (`mine_failclosed_test.go`, com um mock que força a falha do envio) |
| M2 | **Médio** | **Governança: os pesos ignoravam a decisão** — `resolveWeights` aplicava uma tabela fixa do MVP 1.5 (90/10 desde o bloco 1, peso à LLC **extinta**, gatilho por supply) | `x/luggom/govtally/wrap.go` | ✅ **CORRIGIDO** (commit `ebc2eae`, D5) |
| B1 | Baixo | `add-genesis-account` **inflacionava o supply** a cada validador acrescentado (4.000 → 4.400) | `scripts/bootstrap-validators.sh` | ✅ **CORRIGIDO** (§13.141) |
| B2 | Baixo | A tx do oráculo **exigia `--fees` explícito**; sem ele, `insufficient fees` | `cmd/luggom-oracle-feeder` | ✅ **CORRIGIDO** (`-fees`, §13.139) |
| B3 | Baixo | **Pagador em JSON aninhado** fora da janela do nome ⇒ `PayerTaxID` vazio ⇒ **1206 cego** | `api/taxproof/taxproof.go` | ✅ **CORRIGIDO** (§13.138) |
| B4 | Baixo | Params de governança **mortos e travados** (`gov_validator_min_for_weight_shift`, `gov_llc_*`) — o filtro estrito protegia o que ninguém lia | `app/ante_strict_filter.go:146-155` | **ABERTO** — limpeza do Bloco 2 |
| **C1** | **CRÍTICO** | **`MsgMine`/`MsgMineUSD` cunham com ZERO prova** — portões = whitelist de CNPJ + comprimentos; ignoram a fase. Reproduzido: **1.000.000 LUG** com hash inventado | `x/luggom/keeper/sacrifice.go`, `sacrifice_usd.go` | ✅ **FECHADO (§13.147)** — handlers recusam (`ErrMiningMsgDisabled`); as implementações foram **apagadas**; CLI `mine-usd` removido; teste `TestLegacyMineMsgsAreDisabled` (nada é cunhado) |
| **C2** | **CRÍTICO** | **A âncora do anti-replay é escolhida pelo PROVER** — duas provas do mesmo comprovante com redacção diferente dão âncoras **e** `payment_id` diferentes ⇒ mina 2× | `api/taxproof/taxproof.go` (`Anchor`) | ✅ **FECHADO (§13.147)** — o `payment_id` passou a ser **só** o campo canónico (E2E do PIX); o fallback de rótulos foi removido; **fail-closed** sem campo canónico **e** com mais de um E2E distinto. A âncora é o 2.º travão (o docstring foi corrigido) |
| M3 | Médio | **`GetTlsnotaryPhase` devolvia 1** (fase fraca) sem params ou com fase 0 — fail-open. Achado por **ambos** os agentes | `x/luggom/keeper/keeper.go:182` | ✅ **CORRIGIDO** (§13.146) — devolve 0 e a verificação recusa; `InitGenesis` passa a validar |
| M4 | Médio | **`founder_address` VAZIO** deixava os 5% presos no módulo com a tx a reportar sucesso (o vazio é o default). Reproduzido | `x/luggom/keeper/otc_vault.go` | ✅ **CORRIGIDO** + teste (§13.146) |
| M5 | Médio | **PANIC** não autenticado na API (`LuggomAddress[:12]`) — endpoint público | `cmd/luggom-api/main.go` | ✅ **CORRIGIDO** (§13.146) |
| M6 | Médio | **Tokenomics anunciada ≠ chain** (25/25/19,75 vs 30/35/7,5) | `api/uspipeline/tokenomics.go` | ⚠️ **MARCADA desactualizada** — alinhar é decisão de política |

**Herdados do §13.131 (A1–A11):** ✅ **FECHADOS — A3** (§13.144: a âncora passou a ser o **corpo revelado**), **A4** (§13.143: `provider_id` no proto, no **digest v3** e verificado na chain), **A5** e **A10** (§13.145: a fase 1 deixou de ser o default e os **escapes de DEV foram REMOVIDOS do código** — a API já não mina sem notário, responde 503). **Por resolver (produto/dados, não portões):** A1/A2 e os residuais A6–A9/A11. Na matriz T3: T-CHAIN-1/2 e T-API-4/5 ✅ fechados.

### Verificado OK (nesta revisão)

- **Anti-replay nos 3 caminhos de mineração:** `sacrifice.go`, `sacrifice_tax_payment.go` e `sacrifice_usd.go` usam `UsedDocumentHashes` **e** `UsedPaymentIDs` ✓.
- **Pre-sale sem anti-replay — revisto e NÃO é um achado:** comprar mais é legítimo (não há documento a repetir) e o *replay* da própria tx é travado pela **sequência da conta**; o signer é imposto pela anotação `cosmos.msg.v1.signer = "buyer"` ✓.
- **O envio ao cofre OTC é fail-closed**, e o da **fatia do pre-sale** passou a fail-closed no §13.136 ✓.
- **A verificação de atestação da fase 2** (ECDSA + domínio v2 a cobrir todos os campos) tem 4 testes de regressão ✓.
- **O oráculo é fail-closed** (snapshot ausente/stale ⇒ mineração pára) ✓ — corrigido no §13.139.
- **Cobertura:** pre-sale 13 testes, feeder 6, sandbox 3, extrator 6 novos, governança 2, **fail-closed da mineração 4 novos** ✓ · **A/B (§13.147): 4 novos** — `TestLegacyMineMsgsAreDisabled` (a msg não cunha), `TestVerifyTlsnProofBR_NoCanonicalID_FailsClosed` (esconder o E2E não mina), `TestVerifyTlsnProof_MultipleE2E_FailsClosed` (dois ids não escolhem), `TestComprovanteSemCampoCanonico_FalhaFechado` (sandbox) ✓.
- **Testnet local de 3 validadores — VALIDADA:** `scripts/bootstrap-validators.sh local 2` (genesis com fundador + 2, ledger a fechar em 4.000,000204) e `scripts/run-local-testnet.sh start` ⇒ **os 3 nós produzem blocos** (alturas a subir, quórum 2/3+1 fechado), com portas distintas e `persistent_peers` ligados. Fecha o item "3 nós" da prontidão do hard test — e o **nó FULL também foi VALIDADO** (§13.143: 3 validadores + 1 full, todos à mesma altura, o full com `finalized block`/`committed state`, catch-up sem derrubar consenso). **Falta só preencher a matriz T3** — que é o trabalho do hard test.

### ⚠️ Decisão de produto de 2026-09-18 (Fundador) — actualizada

**A mineração passa a ser por DESKTOP.** O **software desktop** (Bloco 5, Tauri) é o **minerador e a carteira**: guarda a **seed** (com backup/restauro, como o APK tinha) e faz a mineração com o prover. O **APK é descontinuado como minerador** e passa a **carteira/transferências** agora — e, **depois da testnet com a comunidade e com tudo validado**, evolui para um **APK de trocas** (mercado), não de mineração. Consequências para este gate:

1. O item do §13.90 *"interface APK ok"* passa a **"interface DESKTOP ok"** — e o **Bloco 5 (Tauri) não existe**. ⇒ É **caminho crítico**, não um bloco de cauda.
2. O **Bloco 3 da FASE 8** (prova falsa, pinning, seed/PIN) passa a ter o **desktop** como alvo. **Sem o desktop, o Bloco 3 não tem alvo.**
3. **A mineração E2E local continua provada** (prover CLI + notário, `code 0`) — o caminho existe; o que falta é **empacotá-lo** no desktop (Bloco 5), com a **carteira e a seed** que o APK tinha.
4. Os vectores de **APK** reduzem-se, no hard test, a **carteira/transferências**.

> **Nota de sequência (DECIDIDA pelo Fundador, 2026-09-18):** construir o **desktop (Bloco 5) PRIMEIRO** — é o maior bloco de construção que falta — para o **Bloco 3 correr COMPLETO** e a FASE 8 fechar inteira. (Alternativa descartada: correr Blocos 1/2/4 e deferir o Bloco 3.) Os vectores **T-ORAC-1..4** do Bloco 3 são de chain e **não** dependem do desktop.

---

*Registo do DV Master — 2026-09-18. Este ficheiro é o registo **do papel**; o gate em si (estados por item + assinaturas) está em `docs/CHECKLIST_FECHO_FASE7.md`.*

---

## 2026-10-07 — Gate: **Fecho do Hard Test Local (FASE 8)**

- **Papel:** DV Master (revisão de qualidade do hard test — código, correcção, edge cases, portões de segurança, cobertura).
- **Escopo revisto:** `relatorioHARDTESTLOCAL.md` (Blocos 1–4) + código de §13.166–§13.174 (A-8, T-GOV-1, §13.170 A–E, vesting, cesta, muro).
- **Verificação no código (não só no relatório):**
  - **A-8 (crítico) — FECHADO:** os créditos às contas de módulo usam `SendCoinsFromModuleToModule` em `x/luggom/keeper/otc_vault.go` (pre-sale, vesting do fundador, reservatórios, OTC, POOL, notário, fee_collector). Sem `ModuleToAccount` para endereço de módulo ⇒ sem panic.
  - **T-GOV-1 (crítico) — FECHADO:** `x/luggom/keeper/params_guard.go` (`validateUpdateParamsProtected`) vive no **msg_server** e compara com os params **atuais** — vale por qualquer caminho, incluindo proposta com a msg embutida. Protege `founder_address` e os pesos do tally da governança.
  - **§13.170 A–E — aplicado:** `types/frozen.go` (Grupo B: `TlsnotaryPhaseFrozen=2`, oráculo sempre ligado, staleness 36h); `api_verifier_address`/`usd_oracle_updater_address` rotáveis; fase 1 **extinta** (constante de código).
  - **Vesting cliff puro 24m — `founder_vesting.go`:** fail-closed (recusa libertação sem `founder_address`); retém em `luggom_founder_vesting` e liberta tudo no cliff via `SendCoinsFromModuleToAccount`. Verificado **ao vivo** na T20 (§3.4 do relatório).
  - **Cesta +10%/ano — `oracle_usd.go`:** `basketMult=3`, `basketAnnualGrowth=1.10`, `mult = 3 × 1.10^years` (determinístico, sem tecto).
  - **§13.174 — muro removido:** `keeper/otc_floor.go`, `types/otc_floor.go` e testes apagados; `RunOtcFloor` fora do `BeginBlock`; conta `luggom_otc_floor` removida. A defesa de preço passa a ser a pool nativa/AMM livre.
- **Suíte:** build dos 3 binários OK (2026-10-07, pós-§13.174). Suíte Go verde por §13.174/§13.172. Reset da chain **feito e validado** (genesis novo, 4 validadores + full a produzir blocos, `founder_address` descartável ≠ multisig de mainnet).
- **Ressalvas (não bloqueiam — decisões/pendências do Fundador):** A-2 (rotas da API sem autenticação — decisão para a testnet); T-API-2/7 (exigem bundle TLSNotary real); `.xpi` do coletor por assinar na AMO; Bloco 2 (notários t-de-n + `dustReserveAddress`).
- **Parecer:** ✅ **APROVADO — nenhum crítico aberto.** O hard test local está **essencialmente fechado** (Blocos 1–4 corridos; T20 happy-path com sessão real → `code 0`; A-8 e T-GOV-1 fechados). O **gate do DV Master fica FECHADO**. Próximo: testnet.

### Achados desta revisão

| # | Gravidade | Achado | Estado |
|---|---|---|---|
| — | Crítico | nenhum novo | — |

*Registo do DV Master — 2026-10-07. Fecha o "Fecho (DV Master)" que estava ⬜ no relatório do hard test.*
