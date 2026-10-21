# Matriz de Vetores T3 — hard test (FASE 8)

> **O que é:** o entregável do hard test (§13.73). O §13.47 registou-a como **lacuna**: *"montar a matriz de vetores T3 no início do hard test — não existe documentada ainda"*. Isto é o **esqueleto**: a estrutura, o formato das linhas e os vetores **já conhecidos**. As linhas em branco são o trabalho do teste.
>
> **⚠️ Actualização §13.147 (2026-09-18):** os **dois CRÍTICOS** do review — **T-CHAIN-13** (`MsgMine`/`MsgMineUSD` sem prova) e **T-CHAIN-14** (`payment_id` escolhido pelo prover) — e os **T-API-8/9** estão **FECHADOS no código** ⇒ deixam de ser "a executar" e passam a **regressão**. **T-CHAIN-1** foi **corrigido** (a âncora não é o travão principal; o id canónico é). **T-GOV-1** foi **rebaixado a parcial** (o filtro estrito é contornável por proposta de gov — falsificado). O **Bloco 3** corre **COMPLETO** após o **desktop (Bloco 5)** — que se constrói **PRIMEIRO** (decisão do Fundador, 2026-09-18; a mineração passa a desktop).
>
> **Estrutura:** as **4 áreas** do §13.73 (os 4 blocos de 2h), que cobrem as 5 áreas de ataque do §13.47.
>
> **Como se preenche:** por vetor — `repro` (comando/passo exacto) · `mitigação` (o que o impede) · `crítico s/n` · `estado`. **Cada achado vira teste de regressão antes de se avançar** (§13.73).

---

## Bloco 1 — API + endpoints (2h)
*Autenticação, `/mine/tax`, `/reclaim/*`, rate-limit, tamper.*

| Vetor | Repro | Mitigação | Crítico | Estado |
|---|---|---|---|---|
| **T1** — bundle E2E real **sem** atestação do notário | submeter o bundle do §13.126 sem `/attestation` | API recusa se o notário não responder | **s** | ✅ **corrido** → `HTTP 400 notary returned 404` |
| **T2** — transcript **adulterado** (1.291,49 → 9.999,99) | alterar o corpo e reenviar | `proof_commitment` não bate | **s** | ✅ **corrido** → `400 proof_commitment mismatch` |
| T-API-1 — `/mine/tax` sem prova / prova vazia | POST com `tlsn_proof` vazio | validação de campos | s | ⬜ |
| T-API-2 — `public_key` ≠ `dest_address` | assinar com outra chave | comparação por bytes (§13.97) | s | ⚠️ **prova real** (bundle com domínio de banco) — teste de integração pronto (`TestHardTest_ApiPublicKeyMismatch_400`, salta sem bundle); check coberto por unidade (`TestPubKeyMatchesAddress`) |
| T-API-3 — `/reclaim/providers` com a chain em baixo | derrubar o nó | fallback para a lista completa (verificar se é desejável) | ? | ⬜ |
| T-API-4 — **escapes de DEV** ligados em produção (A10) | `LUGGOM_TLSN_SKIP_HOST_CHECK=1` + `LUGGOM_FIXTURE_PROVIDER=<banco>` | ✅ **FECHADO** (§13.145): os escapes foram **removidos do código** (não substituídos) — o host check corre sempre | **s** | ✅ **fechado** |
| T-API-5 — `LUGGOM_TAX_ENTITY_MARKER=1` a fixar o ente | corpo com `data-tax-entity` | ✅ **FECHADO** (§13.145): o marcador **nunca** é honrado; o ente sai do CNPJ/EIN/BN do recebedor | s | ✅ **fechado** |
| **T-API-8** — **esconder o campo canónico** (E2E) para forçar o fallback (B) | prova do MESMO recibo **sem a linha do E2E** | ✅ **FECHADO (§13.147)**: o fallback de rótulos **deixou de existir**; sem campo canónico a extracção **falha fechado**. `TestVerifyTlsnProofBR_NoCanonicalID_FailsClosed` | **s** | ✅ **fechado** |
| **T-API-9** — **mais de um E2E** no mesmo comprovante (o prover escolhe o id) | corpo com dois E2E | ✅ **FECHADO (§13.147)**: >1 E2E **distinto** ⇒ falha fechado (o mesmo E2E repetido — chave + `id` da Caixa — conta uma vez). `TestVerifyTlsnProof_MultipleE2E_FailsClosed` | **s** | ✅ **fechado** |
| T-API-6 — rate-limit / abuso do dust | N pedidos de ativação | limite 5/hora/IP — **a ativação só corre DEPOIS de a prova validar** | n | ✅ **fechado por unidade** (5 ok · 6.º recusado; `TestHardTest_ActivationRateLimit_PerIP`) — ver achado **A-6** (XFF) |
| T-API-7 — provider **fora** da whitelist on-chain | `provider_id` não listado | API devolve **403**; e a **chain** recusa com **1215** (§13.143/A4 — a whitelist on-chain manda) | s | ✅ **chain corrida (2026-09-30)** → **`code 1215`** ao vivo; API: teste de integração pronto (`TestHardTest_ApiProviderNotWhitelisted_403`, salta sem bundle real) |

## Bloco 2 — chain/keeper (2h)
*Replay, double-spend, whitelist/blacklist 1206/1207, teto, oráculo stale.*

| Vetor | Repro | Mitigação | Crítico | Estado |
|---|---|---|---|---|
| **T3** — replay de um recibo já minerado | reenviar a mesma tx | `UsedPaymentIDs` | **s** | ✅ **corrido** → on-chain **`code 1202`** |
| **T13** — `payment_date` falsificado (saltar a janela de 365d) | msg com data futura/antiga | digest do §13.122 cobre **todos** os campos | **s** | ✅ **corrido** → rejeitado |
| **T17** — `tax_nature_hash` falsificado | msg sem natureza | idem (A1.4) | **s** | ✅ **corrido** → rejeitado |
| T-CHAIN-1 — `Anchor` **com cabeçalhos** (A3) | re-provar a mesma página → hash diferente | ✅ **FECHADO** (§13.144): a âncora passou a ser o **CORPO revelado** (sem cabeçalhos). ⚠️ **Correcção (§13.146/§13.147):** a âncora **NÃO** é estável quando a redacção difere (o prover escolhe o que revela) — é o **2.º travão**. Quem trava a repetição é o **`payment_id` canónico** (1203) — ver **T-CHAIN-14** | **s** | ✅ **fechado** (via T-CHAIN-14) |
| T-CHAIN-2 — `MsgMineTaxPayment` **sem `provider_id`** (A4) | minar via provider não listado, sem passar pela API | ✅ **FECHADO** (§13.143): a msg tem `provider_id`, **vai no digest (v3)** e a chain verifica-o contra a whitelist governável (1215) — teste de regressão em `bank_provider_test.go` | **s** | ✅ **fechado** |
| T-CHAIN-3 — `TlsnotaryPhase` default = **1** (A5) | chain em fase 1 aceita atestação só por comprimento | ✅ **FECHADO** (§13.145/§13.146): o default dos params é 2 **e** o `GetTlsnotaryPhase` devolve **0** (nunca 1) ⇒ o `default` do switch recusa; o `InitGenesis` valida | **s** | ✅ **fechado** |
| T-CHAIN-4 — **1206** governo↔governo | pagador com ID de ente da whitelist | `payer_tax_id` (§13.135 — **ressuscitado**) | s | ✅ **corrido (2026-09-30)** — chain descartável fase-1 → **`code 1206`** |
| T-CHAIN-5 — **1207** PIX comum (sem natureza fiscal) | PIX para ente da whitelist sem pista fiscal | `tax_nature_hash` | s | ✅ **corrido (2026-09-30)** → **`code 1207`** |
| **T-CHAIN-13** — **`MsgMine`/`MsgMineUSD` sem prova nenhuma** (CRÍTICO do review, §13.146) | submeter `MsgMine` com hash inventado, directo à chain | ✅ **FECHADO (§13.147)** — os handlers **recusam** (`ErrMiningMsgDisabled`); as implementações foram **apagadas**; CLI `mine-usd` removido; `TestLegacyMineMsgsAreDisabled` prova que **nada é cunhado** | **s** | ✅ **fechado** |
| **T-CHAIN-14** — **`payment_id` escolhido pelo PROVER** (CRÍTICO do review, §13.146) | duas provas do MESMO comprovante com redacção diferente; e esconder o campo do E2E | ✅ **FECHADO (§13.147)** — o `payment_id` é **só** o campo canónico (E2E do PIX); o fallback de rótulos foi removido; **fail-closed** sem campo **e** com >1 E2E **distinto**. O mesmo pagamento dá o **mesmo id** ⇒ **1203** (`ErrPaymentIDAlreadyUsed`) | **s** | ✅ **fechado** |
| T-CHAIN-15 — fase 0 fail-open | genesis sem `tlsnotary_phase` | ✅ **FECHADO** (§13.146): `GetTlsnotaryPhase` devolve 0 e a verificação recusa | **s** | ✅ **fechado** |
| T-CHAIN-6 — teto anual | minar acima do cap | janela deslizante 365d | n | ⬜ |
| T-CHAIN-7 — **oráculo stale ⇒ pausa** | deixar o snapshot envelhecer | **fail-closed** (§13.139) | **s** | ⬜ **novo** — testar |
| T-CHAIN-8 — oráculo: snapshot ausente | chain sem snapshot | **fail-closed** (§13.139) | **s** | ⬜ **novo** |
| T-CHAIN-9 — manípular o oráculo com 1 fonte só | feeder com `-min-sources 1` | o feeder **aborta** com <2 (§13.139); a chain julga a mediana | s | ⬜ |
| T-CHAIN-10 — pre-sale: cap / fase / backing | comprar acima do cap, fora da Fase 1, sem custódia | §13.136 (13 testes) | s | ✅ **corrido (2026-09-30)** — `code 0` com `usdt`; **achou o bug A-8** (panic em `BuyPreSale`), corrigido §13.172 |
| T-CHAIN-11 — vesting: libertar acima do total / duplicar no mesmo dia | forçar `RunPreSaleVesting` | `VestedAt` com cap + epoch diário | s | ⏭️ **regressão unitária** (janela de 3 meses não cabe ao vivo) |

## Bloco 3 — Desktop (mineração) / carteira + oráculo (2h)
*Prova falsa, pinning, backup/seed, PIN; cesta, snapshot, guardrails.*

> ⚠️ **A correr COMPLETO — depois do desktop (Bloco 5).** **Decidido (2026-09-18):** construir o **desktop primeiro** (é o maior bloco de construção que falta) para o Bloco 3 ter alvo. A mineração passa a ser por **desktop** (Tauri = **minerador + carteira com seed**); o APK fica **carteira** (e, depois da testnet com a comunidade, **APK de trocas**). Os vectores **T-ORAC-*** são de chain e **podem correr já**. A **mineração E2E já está provada** pelo prover CLI + notário (§13.126/§13.134, `code 0`).

| Vetor | Repro | Mitigação | Crítico | Estado |
|---|---|---|---|---|
| T-APP-1 — prova falsa apresentada pela app (desktop) | bundle forjado | **P0 fechado** (§13.123): a chain verifica a assinatura do notário | **s** | ✅ **corrido (2026-09-30)** — atestação forjada → **`code 1204`** on-chain (2 variantes) |
| T-APP-2 — interceptação TLS (proxy) | MITM no dispositivo | **sem pinning no cliente**; defesa = root store do **notário** (webpki/Mozilla) + **allowlist de domínio** (`ProviderHostAllowed`) | s | ✅ **corrido (2026-09-30)** — teste `TestProviderHostAllowed`; MITM real com CA própria **não** executado (ressalva no relatório §3.5) |
| T-APP-3 — backup/restauro da **seed** (desktop/carteira) | exportar/importar | — | s | ⬜ *(após o Bloco 5)* |
| T-APP-4 — PIN errado N vezes | brute-force no unlock | Argon2id + limite | s | ✅ **corrido (2026-09-30)** — lockout exponencial 5→30 s…3600 s, **persistente** (teste novo `lockout_survives_reopening_the_vault`); Argon2id 64 MiB |
| T-APP-5 — mnemónico exposto em logs/clipboard | inspeccionar | — | s | ✅ **corrido (2026-09-30)** — `SecretPhrase` zera+redige, sem logging da seed, UI limpa, disco só com `ciphertext`; "copiar" = só o endereço |
| T-ORAC-1 — cesta: **perna** fora de `[0.05, 50]` | submeter `MsgUpdateUsdOracleBasket` com perna absurda | `validateOracleFxLeg` | s | ⬜ |
| T-ORAC-2 — cesta abaixo do floor `0.75 × mult` | submeter pernas baixas | `usdParityFromBasketRates` | s | ⬜ |
| T-ORAC-3 — multiplicador: tentar mudá-lo | `MsgUpdateParams` | **constante compilada** (§13.134) | **s** | ⬜ |
| T-ORAC-4 — quem pode alimentar o oráculo | assinar com conta não autorizada | authority ou `usd_oracle_updater_address` — **D3 RESOLVIDO** (§13.139: o endereço do feeder já vai no genesis e foi provado ponta-a-ponta, tx `code 0`) | **s** | ⬜ |

## Bloco 4 — governança + consenso (2h)
*Propostas maliciosas, veto, filtro estrito, listas operacionais; e 3 nós.*

| Vetor | Repro | Mitigação | Crítico | Estado |
|---|---|---|---|---|
| T-GOV-1 — proposta que toca em **tokenomics** | `MsgUpdateParams` a mudar a cesta/o teto | ⚠️ **parcial** — o filtro estrito só vê msgs **de topo**; o x/gov executa as msgs **internas** de uma proposta **sem ante handler** ⇒ um `MsgUpdateParams` **embutido** numa `MsgSubmitProposal` **PASSA** (falsificado no §13.146; o comentário do ficheiro foi corrigido). As **constantes compiladas** (cesta, teto, lote, chão) continuam **inalcançáveis** | **s** | ⚠️ **testar + decisão de desenho** (pré-pool o Fundador tem 100% do peso) |
| T-GOV-2 — proposta a **adicionar ente** à whitelist sem 67% | votar com menos peso | limiar de 67% | s | ⬜ |
| T-GOV-3 — **remover** um banco da whitelist e continuar a minar | `MsgRemoveBankProvider` + minar | ✅ **base FECHADA (§13.143/A4)**: a chain verifica o `provider_id` contra a whitelist **on-chain** (**1215**). *A executar no hard test:* remover por governança e provar que a mineração desse banco **desliga sem deploy** | s | ⬜ *(base fechada)* |
| T-GOV-4 — veto / proposta cancelada a meio | — | — | n | ⬜ |
| T-GOV-5 — consenso a **3 nós**: derrubar 1, forkar 1 | parar/atrasar um validador | tolerância BFT (1/3) | **s** | ⬜ |
| T-GOV-6 — nó **full** atrasado: catch-up | desligar e voltar | state sync / sincronização | n | ⬜ |

---

## Legenda

- **Crítico `s`** — se falhar, a testnet **não** sobe (ex.: permite minar sem imposto, forjar prova, ou parar a chain).
- **Crítico `n`** — relevante mas não bloqueante (DoS local, UX de operação).
- **✅ corrido** — vetor já executado e passado, com a evidência registada.
- **⬜** — a executar no hard test.

## Nota de honestidade

Este esqueleto **não** é o hard test. É o molde e o registo do que **já** está coberto (T1/T2/T3/T13/T17/T20) e do que ficou **fechado** no review (§13.143–§13.147: **A3/A4/A5/A10**, **A/B**, T-CHAIN-13/14, T-API-8/9). O hard test é correr as linhas `⬜`, com **3 validadores + 1 full node**, e transformar cada achado em teste de regressão.

⚠️ **Três notas de arranque:** (1) ✅ **happy-path T20 CORRIDO (2026-09-30)** com **sessão real** — tx `611E02FB…6D3C4D`, altura 1223, **`code 0`** (ver `relatorioHARDTESTLOCAL.md` §3.4); (2) o **Bloco 3** corre **COMPLETO** depois do **desktop (Bloco 5)** — **decidido: construir o desktop primeiro** (mineração por desktop; APK = carteira/trocas); (3) **T-GOV-1** é **parcial** — o filtro estrito não vê msgs internas de uma proposta de governança; é **decisão de desenho** (imutáveis vs governáveis), não bug.
