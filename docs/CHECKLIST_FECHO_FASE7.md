# Checklist de Fecho da FASE 7 — §13.75

> **O que é:** o artefacto que o gate do §13.75 produz. *"O hard test (FASE 8) só
> começa com o checklist **100% ok** ou com **riscos aceitos e documentados**"*
> (§13.75), reforçado pelo §13.90.
>
> **Quem assina:** **DV Master + Fundador**. Sem as duas assinaturas, a FASE 8 não
> abre.
>
> **Como se lê:** cada linha tem um **estado verificado no código/genesis**, não
> copiado do plano. Onde há risco aceito, segue a ficha com **o que fica por
> fazer**, o **risco residual concreto**, a **mitigação actual** e **quando** se
> resolve.
>
> **Data:** 2026-09-18 · **Verificação:** `go test ./x/luggom/... ./api/... ./cmd/luggom-api/... ./app/... ./cmd/sandbox-bank/...` **verde**; os 4 binários reconstruídos.
>
> **⚠️ Actualização §13.147 (mesmo dia):** os **2 críticos** que retinham o parecer — **A** (`MsgMine`/`MsgMineUSD` a cunhar sem prova) e **B** (o `payment_id` escolhido pelo prover) — estão **FECHADOS** (código + binários + 4 testes). O parecer do DV Master volta a **APROVADO** (`docs/CURRENT_STATE.md`). **Este checklist fica a aguardar apenas a assinatura do Fundador** (e a confirmação do D6).

---

## 1. Checklist §13.75 — item a item

| # | Item | Estado | Evidência |
|---|---|---|---|
| 1 | **Fases 0–7 completas** | 🟡 **parcial** | Fases 0–5 ✅; **Fase 6 (pool nativa)** ✅ código, falta a **abertura da pool** (`pool_active=false` no genesis); **Fase 7** (docs/whitepaper/manual §13.111) ✅; **6.5 (2º rail Open Finance)** desenhado e desligado. **6.3/6.4 revistos** no §13.138 |
| 2a | Dívida: **Shamir 7-de-12 + Dead Man's Switch** | ⚠️ **ACEITE COM RISCO** | ver ficha A |
| 2b | Dívida: **feeder do oráculo** + E2E local `/mine/tax` | ✅ **RESOLVIDA** | `cmd/luggom-oracle-feeder` (mediana 3 fontes, fail-closed) + **fail-closed on-chain** (`oracle_usd.go`); **E2E real** feito nos §13.126/§13.134; e o **caminho on-chain do feeder provado** (D3, abaixo) |
| 2c | Dívida: **federais expandidos** | ⚠️ **ACEITE** | ver ficha B — **dados, bloqueado no Fundador** |
| 2d | Dívida: **E2E real Viacredi** | ✅ **RESOLVIDA** | §13.126 / §13.134 (mineração real `code 0`) |
| 3 | **Decisões do Fundador no código** | ✅ **verificado** | tokenomics §13.132 ✅ (cesta ×3, 3 fases, lote 100k, chão $0,03, gov por fase — constantes compiladas); P0/notário ✅ (§13.123); pre-sale on-chain ✅ (§13.136); governança 67% nas listas ✅; blacklist de governos ✅ (1206 ressuscitado §13.135); `tax_nature_hash` ✅; **`usd_oracle_updater_address` = a conta do feeder ✅ (D3 resolvido)** |
| 4 | **Docs ↔ código alinhados** | 🟡 **parcial** | whitepaper/manual §13.111 ✅; **os checkpoints ficaram com números superados** (×6, 35/50, POOL 0,5, muro $0,10) marcados como SUPERADOS no topo — a limpeza fina é trabalho do Bloco 2 |
| 5 | **Refatoração PT→EN completa (§13.67)** | ⚠️ **ACEITE** | ver ficha C |
| 5b | **Reclamações UX do §13.110** (precondição do §13.90) | ⚠️ **ACEITE** | ver ficha D — 3 queixas: "Receber" no extrato, login lento, apagar carteira |
| 6 | **Genesis/bootstrap conforme** | 🟡 **com DIVERGÊNCIAS** | ver §3 — 4.000,000204 ✅ · slashing **70%** ✅ · `mint` removido ✅ · **max_validators = 180 ✅ (D1 RESOLVIDO pelo Fundador)** · **1 validador no genesis local (o checklist diz 20)** · limite de stake 1% ✅ (default) |
| 7 | **Prontidão do hard test** | 🟢 **pronto (falta a matriz)** | **matriz T3**: esqueleto ✅ (`docs/matriz-vetores-T3.md` — o preenchimento é o trabalho do teste). **Nós**: `bootstrap-validators.sh` cria o genesis e `run-local-testnet.sh` arranca os nós — **VALIDADO com 3 validadores a produzir blocos** (quórum 3 fechado, alturas a subir) **e o nó FULL VALIDADO** (§13.143: 3 validadores + 1 full à mesma altura, o full a sincronizar). **Falta**: preencher a matriz (é o hard test) |

**Conclusão do checklist:** **falta apenas a assinatura do Fundador.** Nenhum item exige código novo: as **4 fichas** (A–D) são riscos aceites por decisão; os itens 1/4/6/7 fecham com o teu "ok" (a Fase 6 está completa em código; a limpeza de números é Bloco 2; o **nó full já está validado**, §13.143; os **20 validadores** são setup da FASE 8). Os **2 críticos A/B estão FECHADOS** (§13.147, com 4 testes). **O DV Master emitiu parecer APROVADO** (M1 e A/B fechados; `docs/CURRENT_STATE.md`).

---

## 2. Fichas de risco aceito

### Ficha A — Shamir 7-de-12 + Dead Man's Switch (`x/maintenance-lock`)

- **O que fica por fazer:** o módulo **não existe** (nem há biblioteca Shamir no repositório). O desenho está fixado no **§13.22**: chave-mestra de **patches** em **7-de-12** (1 fundador + 3 devs + 3 auditores + 5 validadores); **heartbeat** de 6 meses; **grace** de +6 m; **após 12 meses sem ping** o pedaço é invalidado e redistribuído por **Proactive Secret Sharing**.
- **Risco residual (concreto):** hoje **não há mecanismo de patch on-chain**. O risco de "patch malicioso" é **nulo por ausência de mecanismo** — mas também **não há sucessão**: se o Fundador desaparecer, não existe caminho de recuperação *on-chain*.
- **Mitigação actual:** a governança on-chain existe e é **separada** (67% para listas operacionais); o **filtro estrito** (`app/ante_strict_filter.go`) existe e barra tocar em tokenomics; a tokenomics crítica está em **constantes compiladas** (governança não a alcança).
- **Quando se resolve:** **pós-testnet**. É o maior código novo do projeto (threshold crypto + heartbeat + redistribuição) e não deve entrar antes de a chain provar que funciona.
- **Nota técnica:** o SDK vendido (`deps/cosmos-sdk`) traz **`x/group` já activo** (multisig ponderado com *decision policies*) — serve para a parte de **votação**, **não** para Shamir/PSS. Serve de base para o que for preciso votar.

### Ficha B — Federais expandidos

- **O que fica por fazer:** expandir as entidades **federais** mineráveis nos 3 países do BCA — **INSS/GPS (BR)** e **departamentos dos EUA**. O registo é explícito: *"Fundador vai passar a lista"* (§13.70 item 6 / §13.75).
- **Risco residual:** entidades federais que deviam ser mineráveis **não o são**; **não há exposição** — é cobertura a menos, não a mais. As entidades já na whitelist funcionam.
- **Mitigação actual:** a whitelist tem os entes já mapeados; minerar uma entidade **fora** da lista é rejeitado (fail-closed).
- **Quando se resolve:** quando houver a lista. É **trabalho de DADOS**, não de código, e o §13.131 já decidiu que a expansão de nomes se faz **depois de um recibo real** (hoje seria adivinhar).

### Ficha C — Refatoração PT→EN (§13.67)

- **O que fica por fazer:** passar identificadores/comentários de PT para EN em **toda** a base. Hoje o código é maioritariamente português (ex.: `usdParityForMineUSD`, `sacrifice_tax_payment.go`, a maior parte dos comentários).
- **Risco residual:** **nenhum funcional** — é legibilidade e revisão. O risco é o **inverso**: um refactor mecânico que toca em tudo **imediato antes do hard test** introduz regressões sem valor de segurança, e o próprio teste passaria a correr sobre código diferente do auditado.
- **Mitigação actual:** os testes cobrem o comportamento; o refactor seria puramente mecânico.
- **Quando se resolve:** **bloco próprio, depois do hard test**. **Este é o item em que o Fundador mais pode discordar** — se preferires fazê-lo antes, é um bloco dedicado (grande e mecânico).

### Ficha D — Reclamações UX do §13.110

- **O que fica por fazer:** as **3 queixas** registadas — (1) o extrato **não mostra "Receber"**; (2) **login/desbloqueio lento** (Argon2id); (3) **apagar carteira não funciona**.
- **Risco residual:** nenhum de segurança — são **fricções de uso**. O risco concreto é de **adoção** (o §13.110 diz-se para diagnosticar por telemetria e transformar em teste de regressão).
- **Mitigação actual:** estão **documentadas e reproduzíveis**; nada nelas permite perder fundos ou minar indevidamente.
- **Quando se resolve:** são **correcções do APK**, independentes da chain. Podem ser feitas **em paralelo** com o hard test; a decisão de as aceitar agora é sobre **quando** as fazer, não sobre as fazer.
- **Nota:** o §13.90 exige *"interface APK ok + mineração testada E2E local"* — a **mineração E2E local** já está provada (§13.126/§13.134); a "interface ok" deve ser confirmada por ti, que és quem usa o APK.

---

## 3. Divergências encontradas na verificação (a tratar)

| # | O que diz o checklist/plano | O que está no código/genesis | Nota |
|---|---|---|---|
| D1 | "MaxValidators **250**" | genesis: `max_validators = **180**` | ✅ **RESOLVIDO (Fundador, 2026-09-18): 180.** O texto do §13.75 está **desactualizado** — o 250 vem do **§13.61**, e o **§13.116** baixou para **180** (os scripts `bootstrap-genesis.sh`/`prepare-e2e-real.sh` já dizem *"180 (antes 250, §13.61)"*). **Nada a mudar no código** — só o texto do §13.75 |
| D2 | "**20 validadores**" no bootstrap | genesis **local**: `gen_txs = **1**` | esperado num genesis de dev; a testnet precisa dos 20. **Não** é falha, é o passo "setup de nós" da FASE 8 |
| D3 | (não estava no checklist) | `usd_oracle_updater_address = **""**` | ✅ **RESOLVIDO (2026-09-18).** O feeder corria com conta normal e seria **recusado** (só a authority alimentava). Agora: `prepare-e2e-real.sh` escreve o endereço do feeder no genesis e **funda-o com 50 LUG tiradas do fundador** (o supply total **não muda** — verificado: 4.000,000204). 🔑 A conta foi criada pelo **Fundador** (`oracle-feeder`) — a chave privada **nunca** passou pelo agente, só o endereço `luggom1kppws…`. **Provado ponta-a-ponta** na chain local: tx `code 0`, altura 20, evento `luggom_usd_oracle_update` com as taxas da mediana, e a fee fixa de 0,005 LUG debitada |
| D4 | "docs ↔ código alinhados" | checkpoints com números superados | marcados como SUPERADOS no topo; limpeza fina = Bloco 2 |
| **D5** | (apanhado pelo Fundador) | **`resolveWeights` ignorava os params** de governança e aplicava uma tabela fixa do MVP 1.5 (0/9000/1000 desde o bloco 1, peso à LLC **extinta**, mudança por **supply**), enquanto o filtro estrito protegia os params que ninguém lia | ✅ **CORRIGIDO (2026-09-18).** Pré-pool `0/10000/0` (params) · pós-pool **`0/3300/6700`** (Fundador 33% / validadores 67%) em **constantes compiladas**, gatilho na **abertura da pool**; defaults alinhados; testes novos. **Era bloqueador** — o Bloco 4 da FASE 8 ia testar pesos errados |
| **D6** | *"interface APK ok"* (§13.90) | **Decisão de produto do Fundador (2026-09-18, actualizada): a mineração passa a ser por DESKTOP.** O **software desktop** (Tauri) é o **minerador + carteira** (guarda a **seed**, com backup/restauro — o que o APK tinha). O **APK é descontinuado como minerador**: fica **carteira/transferências** agora e, **depois da testnet com a comunidade e com tudo validado**, evolui para **APK de trocas** (mercado) | ⚠️ **MUDA O GATE.** O item passa a *"interface **desktop** ok"* — e o **Bloco 5 (Tauri) não existe** ⇒ é **caminho crítico**, não um bloco de cauda. O **Bloco 3 da FASE 8** (prova falsa/pinning/seed) muda de alvo para o **desktop**; sem ele o Bloco 3 **não tem alvo**. Os vectores de APK reduzem-se a **carteira**. **DECIDIDO (2026-09-18): construir o desktop (Bloco 5) ANTES** — assim o **Bloco 3 corre completo** e a FASE 8 fecha inteira. Nota: os vectores **T-ORAC-1..4** são de chain e não dependem do desktop |

---

## 4. Estado — e o que falta para o gate ABRIR

**Já resolvido desde a primeira versão deste checklist:** **D1** (180, decidido) · **D3** (feeder nos params, **provado ponta-a-ponta**) · **D5** (pesos da governança corrigidos) · **B1–B3** (supply dos validadores, fee do oráculo, pagador aninhado) · **M1** (4 envios fail-closed) · **A3/A4/A5/A10** (§13.143–§13.145) · **os 2 críticos A/B** (§13.147). **O parecer do DV Master é APROVADO** — `docs/CURRENT_STATE.md`.

**Falta apenas a assinatura do Fundador.** Nada de código:

1. **Fichas A–D** — aceitar (ou mandar executar).
2. **Item 1** — confirmar *"Fases 0–7 completas"*: a **Fase 6 (pool)** está **completa em código** (a abertura é operacional); o texto do §13.75 fala de *"Fase 7 FPT"*, mas o **FPT/Farol foi ELIMINADO** (§13.135) — essa parte está morta.
3. **Item 4** — aceitar que a limpeza fina de números superados é o **Bloco 2**.
4. **Item 6/7** — aceitar que **os 20 validadores** (D2) são setup da FASE 8. Os **3 nós + 1 full** já estão **validados** (§13.143) e a **matriz T3** é o trabalho do teste.
5. **D6** — a **mineração passa a desktop**: o desktop (Tauri) é o **minerador + carteira (seed)**; o APK fica **carteira** agora e **trocas** depois da testnet. O Bloco 3 muda de alvo e o **Bloco 5 (Tauri) entra no caminho crítico**. **DECIDIDO:** construir o **desktop primeiro** (é o maior bloco de construção que falta) ⇒ o **Bloco 3 corre completo** e a FASE 8 fecha inteira.

---

## 5. Assinaturas do gate (§13.75)

**FUNDADOR** — riscos aceitos e fecho:

| Decisão | Escolha |
|---|---|
| Ficha A — Shamir 7-de-12 + DMS → pós-testnet | **✅ aceito** (pós-testnet) |
| Ficha B — federais expandidos | **✅ aceito** (a lista fica para quando houver dados/recibo real) |
| Ficha C — PT→EN (§13.67) | **✅ aceito** (pós-hard test) |
| Ficha D — UX §13.110 (3 queixas) | **✅ aceito** (o "Receber" e o "apagar carteira" passam a resolver-se no **desktop**, D6) |
| Item 1 — Fases 0–7 completas (pool fechada; FPT eliminado) | **✅ ok** |
| Item 4 — docs↔código (limpeza = Bloco 2) | **✅ aceito** |
| Item 6 — genesis (180 ✅ · 20 nós = FASE 8) | **✅ aceito** |
| Item 7 — prontidão (matriz ✅ esqueleto · **3 nós + full VALIDADOS**, §13.143) | **✅ aceito** |
| **D6 — mineração por desktop; APK só carteira (e trocas pós-testnet)** | **✅ confirmo** |

**Data:** 2026-09-18 · **Assinatura:** Fundador — *aprovado por instrução directa do Fundador (registado pelo agente, 2026-09-18)*

**DV MASTER** — parecer em `docs/CURRENT_STATE.md` (**APROVADO**). *Fechados desde a 1.ª versão:* **M1** (4 envios fail-closed) · **A3/A4/A5/A10** (§13.143–§13.145) · **os 2 críticos A/B** (§13.147). *Abertos por decisão:* **B4** (limpeza do Bloco 2) e **M6** (tokenomics exposta na API — política). Nenhum crítico nem médio aberto nos caminhos de mineração.

**Data:** 2026-09-18 · **Assinatura:** DV Master — parecer **APROVADO** (A/B fechados, §13.147)

---

## 6. Fecho

> ✅ **A FASE 7 ESTÁ FECHADA** — as **duas assinaturas** foram dadas em **2026-09-18**.
> A **FASE 8 abre** com o **Bloco 5 (desktop)** primeiro (decisão D6) e depois o
> **hard test local**. Roteiro: **`docs/PLANO_DESKTOP_E_HARDTEST.md`**.

---

**Próximo passo (FASE 8):** construir o **desktop (Bloco 5)** — ver `docs/PLANO_DESKTOP_E_HARDTEST.md` — e depois subir o ambiente do hard test: notário (`:7047`, com a chave), genesis confirmado (`tlsnotary_phase=2` ✅, `api_verifier_address` ✅), chain + API, **3 validadores + 1 full** (já validado, §13.143), feeder, e a **matriz T3** a preencher nos **4 blocos × 2h** (gate do Fundador entre blocos). ⚠️ **Duas notas de arranque:** (1) o **happy-path (T20)** já **não** corre por fixture (§13.145) — exige **sessão real** ou o fixture servido **num domínio de banco**; (2) o **Bloco 3** corre **COMPLETO** depois do desktop. **Estado: FASE 7 FECHADA (2026-09-18).**
