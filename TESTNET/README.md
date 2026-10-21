# TESTNET/ — pacote público da testnet do Luggom

Tudo o que é **publicável** para rodar, validar e notariar a testnet. Esta pasta
é pública (GitHub / webpage / VPS dos validadores): **nunca** coloque aqui chaves,
seeds, keyrings, caminhos locais ou nomes reais.

Quatro fluxos, nesta ordem:

1. [Rodar um nó / validador](#1-rodar-um-nó--validador)
2. [Ser notário](#2-ser-notário)
3. [Minerar pela carteira desktop](#3-minerar-pela-carteira-desktop)
4. [Adicionar um banco (developer guide)](DEVELOPER_GUIDE.md) · [EN](DEVELOPER_GUIDE.en.md)

> **Roadmap / estado do lançamento:** [ROADMAP.md](ROADMAP.md) · [EN](ROADMAP.en.md)

---

## Pré-requisitos

- Go (toolchain local: `export PATH="$PWD/.go1.23.8/bin:$PATH"`).
- Rust (para o notário).
- Os binários em `build/`: `luggomd`, `luggom-api`, `luggom-extract`.

Build:

```sh
export PATH="$PWD/.go1.23.8/bin:$PATH"
go build -ldflags="-checklinkname=0" -o build/luggomd       ./cmd/luggomd
go build -ldflags="-checklinkname=0" -o build/luggom-api     ./cmd/luggom-api
go build -ldflags="-checklinkname=0" -o build/luggom-extract ./cmd/luggom-extract
```

---

## 1. Rodar um nó / validador

### 1.1 Genesis (coordenador)

O coordenador gera o genesis e fixa os parâmetros (max_validators=180, slashing
70% de uptime, gas mínimo 1uluggom, `founder_address` e `api_verifier_address`).

```sh
# A chave do notário tem de estar definida ANTES do genesis (grava api_verifier_address).
export LUGGOM_NOTARY_KEY_HEX="<chave-hex-do-notário>"
./scripts/prepare-e2e-real.sh --chain-id luggom-test
```

Sem `LUGGOM_NOTARY_KEY_HEX`, a chain fica sem verificador e **recusa mineração**
(fail-closed) — só serve para inspecionar. Para minerar, defina a chave e exporte
`LUGGOM_NOTARY_URL`.

### 1.2 Validadores (modo testnet)

Na testnet, **cada validador cria a própria conta** e envia o gentx. O coordenador
funde os endereços e recolhe os gentx.

```sh
# validators.txt — um endereço por linha (opcional: "luggom1… 200" para bond)
./scripts/bootstrap-validators.sh external validators.txt --gentx-dir ./gentxs
```

Cada validador, do lado dele, produz o gentx:

```sh
luggomd genesis gentx <chave> 200000000000000000000uluggom --chain-id luggom-test --home $HOME/.luggom
```

> ⚠️ O modo `local N` é **descartável** (gera as chaves e imprime os mnemonics) —
> nunca usar na testnet. A testnet usa sempre `external`.

### 1.3 Subir os nós

```sh
./scripts/run-local-testnet.sh start     # arranca todos em background
./scripts/run-local-testnet.sh status    # confere a altura de cada nó
./scripts/run-local-testnet.sh stop
```

Portas (por nó, incrementadas de 10 em 10):

| Serviço | Porta base |
|---|---|
| p2p | `26656` |
| rpc | `26657` |
| LCD (REST) | `1317` |
| gRPC | `9090` |

> ⚠️ **RESET obrigatório antes da testnet**: a chain local atual tem o estado
> pré-A-8 (contas de módulo `BaseAccount`) — minerar com o binário novo faria
> panic. O `prepare-e2e-real.sh` já recria o genesis do zero.

---

## 2. Ser notário

O notário é o serviço TLSNotary que termina a conexão TLS fora do navegador,
extrai o comprovante e **assina** a atestação (ECDSA secp256k1). A chain só cunha
com uma assinatura válida.

### 2.1 Build

```sh
cd tlsn-verifier/servers
OPENSSL_DIR=/usr OPENSSL_LIB_DIR=/usr/lib/x86_64-linux-gnu OPENSSL_INCLUDE_DIR=/usr/include \
  cargo build --release -p tlsn-verifier-server
# → tlsn-verifier/servers/target/release/tlsn-verifier-server
```

### 2.2 Chave do notário

```sh
# derivar o endereço da chave (é este endereço que vai ao genesis como api_verifier_address):
./build/luggom-extract --print-address "$LUGGOM_NOTARY_KEY_HEX"
```

A mesma `LUGGOM_NOTARY_KEY_HEX` tem de estar definida **antes** do `prepare-e2e-real.sh`
(§1.1), para o genesis gravar o endereço certo.

### 2.3 Subir

```sh
cd tlsn-verifier/servers/verifier
LUGGOM_NOTARY_KEY_HEX="<chave-hex>" \
LUGGOM_EXTRACTOR_BIN="$PWD/../../../build/luggom-extract" \
  ../target/release/tlsn-verifier-server
```

Escuta em `0.0.0.0:7047` (`/health`, `/info`, `/session`, `/verifier`, `/proxy`,
`/attestation`). Sem `LUGGOM_NOTARY_KEY_HEX` o notário **não assina** e a API
rejeita a mineração.

### 2.4 Ligar a API

```sh
export LUGGOM_NOTARY_URL=http://127.0.0.1:7047
export LUGGOM_KEYRING=GGOM
./scripts/run-api.sh    # API em :8082
```

> Arquitetura do notariado: **1 notário no arranque → 3 (2 de 3) → 5 (3 de 5)**,
> planeado para depois da mainnet. Na testnet o notário é único.

---

## 3. Minerar pela carteira desktop

O minerador é uma aplicação desktop (Tauri), a carteira vive no bolso.

- Instaladores Linux: `.deb` e `.AppImage` (gerados por `cargo build -p luggom-desktop` / `tauri build`).
- Abas **Carteira · Minerar · Rede**.
- A aba **Rede** aponta para o LCD do nó (`http://127.0.0.1:1317`).
- A aba **Minerar** aponta para a API (`http://127.0.0.1:8082`) e o notário (`http://127.0.0.1:7047`).
- A chave privada **nunca** sai da máquina; a mineração é feita no desktop.

### Ativação de carteira nova (o pó)

Uma carteira acabada de criar não existe on-chain e não paga taxa. A API envia um
**pó de ativação** **uma vez** por endereço, depois de validar a prova TLSNotary:

- Pó = `1e7 uluggom` = **10× a taxa de mineração** (`1e6 uluggom`).
- A conta fica financiada para **10 tentativas** (cada tentativa, mesmo falhada,
  gasta a taxa).
- **Se minerar com sucesso à 1.ª**, recebe o LUG minerado e o pó deixa de ser
  relevante — não precisa de mais nada.
- **Se falhar 10 vezes** sem nunca minerar, o saldo esgota e a carteira fica
  presa (a API não reenvia o pó). Destravar = enviar LUG manualmente para o
  endereço.

> ⚠️ **A rever na testnet** (achado do Passo E): o pó é one-shot por endereço e
> finito (10 taxas). A política certa — reenviar após X falhas ou deixar preso —
> é uma decisão aberta.

O logo da carteira usa o mesmo "L" do site (ver `../site/assets/img/mark*.png` e
`wordmark.svg`).

---

## 4. Adicionar um banco (developer guide)

Como a comunidade transforma um banco/portal de impostos em um rail minerável —
**evidência → sonda → código → governança**:

- [DEVELOPER_GUIDE.md](DEVELOPER_GUIDE.md) (PT)
- [DEVELOPER_GUIDE.en.md](DEVELOPER_GUIDE.en.md) (EN)
- [item153_Spec.md](item153_Spec.md) (PT) · [EN](item153_Spec.en.md) — domínio do banco on-chain

O passo a passo detalhado (as 6 perguntas, a sonda `verify_rail.py`, a checklist de
9 pontos de código, limites técnicos e privacidade) vive nesses dois ficheiros. A
contribuição da comunidade é o **pacote de evidência** (recibo real sanitizado +
campo canónico); o merge de código e a whitelist on-chain passam pela governança.

---

## Regras de segurança

- **NUNCA** colocar aqui: chaves, seeds, keyrings, caminhos locais, nomes reais.
- `genesis.json` real e binários são gerados em runtime — esta pasta guarda
  instruções e scripts, não artefatos nem segredos.
- `MAINNET/` (com `restrito/`) é separada e **nunca** é pública.
