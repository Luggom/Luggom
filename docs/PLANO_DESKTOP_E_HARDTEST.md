# Plano — Desktop (Bloco 5) → Hard test local (FASE 8)

> **Objectivo:** ficar com **tudo pronto para o hard test local** — o **desktop**
> (minerador + carteira com a seed) construído, a FASE 7 assinada, o ambiente
> montado e a **matriz T3** pronta a preencher.
>
> **Decisão do Fundador (2026-09-18):** o **desktop constrói-se PRIMEIRO** — é o
> maior bloco de construção que falta e é o **alvo do Bloco 3** do hard test
> (`T-APP-1..5`). Só depois se corre a FASE 8 inteira.
>
> **Fonte de verdade:** os `checkpoint*.md` e `docs/CURRENT_STATE.md`. Este plano é
> o **roteiro de execução**; onde houver conflito, manda o checkpoint.
>
> **Data:** 2026-09-18 · **Rust:** `cargo 1.98.1` presente · **Go:** `./.go1.23.8/bin`.

---

## 0. O que já existe (não se constrói de novo)

| Peça | Onde | Estado |
|---|---|---|
| **Assinador Rust real** (o que minerou on-chain) | `apk_luggom/native/tlsn-mobile/src/signer.rs` + `examples/sign_tx.rs` | ✅ funciona |
| **Prover** (MPC-TLS) | `apk_luggom/native/tlsn-mobile/src/prover.rs` + `ws_io.rs`; CLI `tlsn-verifier/packages/prover-ws-poc` | ✅ funciona (E2E §13.126/§13.134) |
| **Notário** (extrai + assina) | `tlsn-verifier/servers/verifier` (`:7047`) | ✅ funciona |
| **Chain + API + feeder** | `build/luggomd`, `cmd/luggom-api`, `cmd/luggom-oracle-feeder` | ✅ binários reconstruídos |
| **Scripts** | `scripts/prepare-e2e-real.sh`, `run-local-testnet.sh`, `bootstrap-validators.sh`, `bootstrap-genesis.sh`, `run-api.sh` | ✅ validados |
| **Carteira (UI herdada)** | `apk_luggom/lib/` (Flutter) | só como **referência de UX** — o desktop é Tauri |

> 🔑 **Porque o desktop é viável:** o **assinador e o prover já são Rust**. O desktop
> (Tauri) é Rust por baixo ⇒ **reaproveita-os**; o trabalho novo é **UI +
> armazenamento da seed + empacotamento**, não cripto.

---

## FASE A — Desktop (Bloco 5)  ← *começar por aqui*

### A1. Scaffold Tauri + ligar o crate `tlsn-mobile`
- **Fazer:** app Tauri (Rust + webview) que compila; o backend Rust depende do
  `tlsn-mobile` (signer + prover) e expõe 2–3 comandos de teste.
- **Risco de ambiente:** o Tauri precisa de **webkit2gtk** no Linux (e do toolchain
  do SO no Windows). Confirmar antes de escrever UI.
- **Aceite:** `cargo tauri dev` abre a janela; uma chamada de teste ao `signer.rs`
  devolve um endereço bech32 `luggom1…`.

> ⚠️ **BLOQUEIO RESOLVIDO (2026-09-19).** O ambiente foi instalado
> (`sudo apt-get install -y pkg-config libwebkit2gtk-4.1-dev libgtk-3-dev librsvg2-dev libayatana-appindicator3-dev`)
> e o A1 foi **fechado**: a janela abre e o `signer_probe` devolve o endereço do KAT
> contra o `luggomd` — confirmado pelo Fundador na máquina, não só em teste (§13.150).
> **Só o crate `tauri` precisa do `webkit2gtk`** — `core` e `wallet` testam-se sem ele.
> Estrutura: `desktop/core`, `desktop/wallet`, `desktop/src-tauri`, `desktop/ui`.
>
> 🔤 **Requisito do Fundador: a UI tem de estar em PORTUGUÊS e INGLÊS, com seletor.**
> Vale para **todos** os passos seguintes do desktop (A3, A4, A6) — texto novo entra
> nos dois idiomas em `desktop/ui/src/i18n.ts`, nunca só num. As mensagens de erro
> seguem o mesmo caminho: o Rust devolve `kind` e a UI traduz.
>
> ⚠️ **Armadilha do npm:** com `NODE_ENV=production` o npm salta as devDependencies.
> Já fixado em `desktop/ui/.npmrc` (`include=dev`) — não remover.
>
> ⚠️ **`tauri dev` em `debug` procura o Vite em `localhost:5173`.** Lançar pelo caminho
> oficial (a CLI arranca o `beforeDevCommand`); correr o binário à mão dá
> "Connection refused". O diretório de trabalho do `beforeDevCommand` é `src-tauri`.

### A2. **Seed** — criar/importar/guardar/backup (o mais sensível)
> É o vector **T-APP-3** e o maior risco de segurança do desktop. Desenhar **agora**,
> não depois.
- **Fazer:**
  - criar carteira nova (mnemónico BIP39) e **importar** mnemónico existente;
  - **guardar cifrada** (chave derivada do **PIN** por **Argon2id**), no **keychain
    do SO** como fallback de conveniência (nunca seed em claro no disco);
  - **backup/restauro**: exportar de forma explícita (ficheiro protegido), importar
    e recuperar; e **apagar** a carteira (o APK tinha aqui uma queixa, §13.110/D — a
    resolução passa a valer para o desktop);
  - **PIN errado N×**: atraso/limite.
- **Aceite:** seed nunca aparece em claro (nem em logs/clipboard — **T-APP-5**);
  exportar→apagar→importar devolve o **mesmo endereço** e o mesmo saldo.

> ✅ **FEITO em 2026-09-19 (`desktop/core`).** BIP39 24 palavras · cifra
> **XChaCha20-Poly1305** com chave do **PIN por Argon2id** (64 MiB, t=3) · cabeçalho
> **amarrado ao texto cifrado por AAD** (baixar o `m_cost` invalida o ficheiro) ·
> **sal novo** no backup · ficheiro **0600** + escrita atómica · `Debug` redigido +
> `zeroize` · travão de PIN (5 → 30 s a duplicar até 1 h — **travão de UX, não fronteira**).
> **24 testes** (`cargo test -p luggom-core`) + o aceite **exportar→apagar→importar =
> mesmo endereço** em `desktop/wallet/tests/vault_backup_restore.rs` (**2 testes**), com
> KAT contra o `luggomd`.
> **Falta do A2:** integração com o **keychain do SO** (a cifra em ficheiro está feita;
> o keychain era "fallback de conveniência") e a UX de exportação no clipboard.

### A3. Carteira — saldo, envio, extrato
- **Fazer:** ler saldo on-chain; **enviar LUG→LUG**; extrato com "Receber" visível
  (corrige a queixa §13.110 no desktop); taxas fixas (0,005 LUG) e o pó (§13.10).
- **Aceite:** enviar numa chain local (3 validadores) e ver o saldo a mexer; a fee
  é a esperada.

### A4. Mineração — embutir o prover + UI
- **Fazer:** UI "impostos elegíveis → Minerar"; o **prover** corre contra o **notário**
  (`LUGGOM_NOTARY_URL`) e produz o bundle; a app assina a tx com o **assinador real**
  e faz broadcast (fluxo BLOCO 4: a API monta com `--generate-only`, o APK/desktop
  assina localmente — a seed nunca sai).
- **Aceite:** **1 mineração real E2E** pela app (Viacredi ou fixture servido **num
  domínio de banco**) ⇒ `code 0`. É o mesmo caminho já provado pelo CLI, agora
  empacotado.

### A5. Empacotamento
- **Fazer:** **Windows `.msi`/`.exe`** (mais fácil), **Linux `.deb`/AppImage**;
  **SHA-256** publicado (regra §13.12).
- **Aceite:** instalar num SO limpo, abrir, importar a seed do A2 e ver o saldo.

### A6. Instalador do fluxo de suporte
- **Fazer:** instruções curtas (minerar, backup da seed, o que fazer se o notário
  não responder) — a UX do §13.110 no desktop.

**Mapa desktop → vectores do Bloco 3:** A2 ⇒ `T-APP-3/5` · A4 ⇒ `T-APP-1/2` (prova
falsa/pinning) · A3 ⇒ carteira/transferências.

---

## FASE B — Setup do hard test (depois do desktop)

1. **Notário de pé:** `tlsn-verifier/servers/verifier` em `:7047` com
   **`LUGGOM_NOTARY_KEY_HEX`** (endereço dev: `luggom1l3e9pgs3mmwuwrh95fecme0s0qtn2880wzfw0s`).
   Build Rust precisa de `OPENSSL_DIR=/usr OPENSSL_LIB_DIR=/usr/lib/x86_64-linux-gnu
   OPENSSL_INCLUDE_DIR=/usr/include` (falta `pkg-config`).
2. **Genesis:** `scripts/prepare-e2e-real.sh` — escreve `tlsnotary_phase=2`,
   `api_verifier_address` = notário, e o `usd_oracle_updater_address` (D3).
   **Confirmar:** supply **4.000,000204**, `max_validators=180`, `mint` removido,
   slashing 70%.
3. **Nós:** `scripts/bootstrap-validators.sh local 2` (fundador + 2) e
   `scripts/run-local-testnet.sh start` ⇒ **3 validadores + 1 nó full**
   (`prep-full`). ⚠️ **quórum = 3**: se um cai, a chain **para**.
4. **API:** `scripts/run-api.sh` com o notário no ar (sem notário, `/mine/tax` → **503**).
5. **Feeder:** `cmd/luggom-oracle-feeder` (senão a mineração **pausa** — é fail-closed).

**Aceite da FASE B:** os 4 nós a produzir blocos, o nó full a sincronizar, a API a
responder e o feeder a alimentar o oráculo.

---

## FASE C — Hard test (FASE 8)

- **Formato:** **4 blocos × 2h** (1: API · 2: chain/keeper · 3: desktop/oráculo ·
  4: governança/consenso), com **gate do Fundador entre blocos** e **gate do DV Master
  no fim**. **Cada achado vira teste de regressão antes de avançar.**
- **Entregável:** **`docs/matriz-vetores-T3.md` preenchido**.
- ⚠️ **`T20` (happy-path)** já **não** corre por fixture (§13.145): **sessão real**
  (Viacredi) ou o fixture servido num **domínio de banco**.
- ⚠️ **`T-GOV-1` é parcial** (o x/gov contorna o filtro estrito) — a decisão de
  desenho é do Fundador.

---

## Decisões / riscos abertos (levar para o dia)

| # | Questão | Dono | Impacto |
|---|---|---|---|
| 1 | Assinatura do **Fundador** no `docs/CHECKLIST_FECHO_FASE7.md` | Fundador | **abre** a FASE 8 |
| 2 | Fichas **A–D** (Shamir, federais, PT→EN, UX) — aceitar | Fundador | fecho da FASE 7 |
| 3 | `T-GOV-1`: imutáveis vs governáveis | Fundador | desenho |
| 4 | **Seed no desktop**: keychain do SO + Argon2id — confirmar o desenho do A2 | Fundador + DV | segurança do desktop |
| 5 | `api/`/`docs/`/`scripts/` **fora do Git** + **sem remoto** | Fundador | backup é o tarball (copiar para fora!) |

---

## Checklist — "pronto para o hard test" ☐

- [x] **A1** scaffold Tauri abre e chama o `signer.rs` *(confirmado pelo Fundador na máquina: janela + `signer_probe` + criação de carteira)*
- [x] **A2** seed: criar/importar/guardar(cifrada)/backup/restauro/apagar + PIN
- [x] **A3** carteira: saldo/envio/extrato numa chain local *(provado a 3 validadores: custo real = valor + gás + 0,005 LUG)*
- [ ] **A4** **mineração E2E pela app** ⇒ `code 0`
- [ ] **A5** instaladores + SHA-256
- [ ] **B1** notário `:7047` de pé com a chave
- [ ] **B2** genesis conforme (fase 2, verifier, feeder, 4.000,000204, 180)
- [ ] **B3** 3 validadores + 1 full a produzir/sincronizar blocos
- [ ] **B4** API + feeder a funcionar
- [ ] **C0** `docs/matriz-vetores-T3.md` aberto e pronto a preencher
- [ ] **Assinatura** do Fundador (e parecer do DV já APROVADO)

---

## Comandos de referência

```bash
export PATH="$PWD/.go1.23.8/bin:$PATH"                 # toolchain Go do projeto
go build -ldflags="-checklinkname=0" -o build/luggomd ./cmd/luggomd   # (+ luggom-api)
go test ./x/luggom/... ./api/... ./cmd/luggom-api/... ./app/... ./cmd/sandbox-bank/...

# FASE B
scripts/prepare-e2e-real.sh
scripts/bootstrap-validators.sh local 2
scripts/run-local-testnet.sh start|stop|status
scripts/run-api.sh
```
