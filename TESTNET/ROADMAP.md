# Roadmap — Testnet Luggom

O que falta para a testnet abrir, quem faz o quê, e o que a comunidade pode
assumir **já**. É público: serve de **checklist de lançamento** para o Fundador e
de **mapa de contribuição** para quem chegar.

> **Atualizado:** 2026-10-08. Estado do código em `docs/CURRENT_STATE.md` e
> `checkpoint04.md` (internos).

---

## 1. O objetivo da testnet

Ter uma rede **pública** onde a comunidade **valida, notaria, minera e adiciona
bancos** sem depender do Fundador. Hoje o código está pronto e validado localmente
(T20 com sessão real → `code 0`); o que falta é **lançar e destravar o
self-service**.

---

## 2. Checklist de lançamento (Fundador — nesta ordem)

> ⚠️ A ordem importa. A chave do notário tem de entrar **no genesis** — se subires
> os nós antes, tens de fazer reset de novo.

1. **Fixar a chave do notário** (`LUGGOM_NOTARY_KEY_HEX`) — decidir a chave real.
2. **Gerar o genesis** com o endereço do notário:
   `./scripts/prepare-e2e-real.sh --chain-id luggom-test` (com a variável acima).
3. **Subir os validadores** em modo `external` (nunca `local`):
   `./scripts/bootstrap-validators.sh external validators.txt --gentx-dir ./gentxs`.
4. **Subir o notário** (`tlsn-verifier-server`, §2 do README).
5. **Subir a API** (`./scripts/run-api.sh`) e o **oráculo** (feeder + proxy).
6. **Expor na VPS** — endpoints públicos (não `127.0.0.1`): RPC `:26657`,
   LCD `:1317`, API `:8082`, notário `:7047`.
7. **Assinar o `.xpi` na AMO** (`./scripts/build-collector-xpi.sh --sign`).
8. **Hospedar os instaladores** (`.deb` + `.AppImage` + `SHA256SUMS`) na webpage.
9. **Publicar no GitHub** — incluindo os `docs/` que hoje vivem só no tarball
   (`adapter-spec.md`, `CHECKLIST_FECHO_FASE7.md`, `matriz-vetores-T3.md`).
10. **Instaladores Windows/macOS** (hoje só Linux; `tauri.windows.conf.json` pronto).

---

## 3. O que a comunidade pode assumir já

| Contribuição | Como | Doc |
|---|---|---|
| **Rodar validador** | criar a conta, gerar o gentx, subir o nó | `README.md` §1 |
| **Ser notário** | subir o `tlsn-verifier-server` com a chave | `README.md` §2 |
| **Minerar** | instalar o app desktop e provar pagamento | `README.md` §3 |
| **Adicionar um banco** | entregar pacote de evidência (recibo sanitizado + campo canónico) | `DEVELOPER_GUIDE.md` |
| **Testar Windows/macOS** | keychain + instaladores (só Linux validado) | — |
| **Ensinar a minerar (GoldHunters)** | onboarding de pessoas/empresas | site/whitepaper |

**A contribuição de maior alavancagem agora:** os **pacotes de evidência** para
**US/CA** (e bancos BR novos) — os rails US/CA estão desligados de propósito à
espera do **campo canónico por rail**, e isso é **dado real**, não código.

---

## 4. O que NÃO é alçada da comunidade (depende de código/decisão)

- **Item 153** — adicionar banco **sem re-assinar o app** (domínio on-chain + coletor dinâmico). É o maior destravador de autonomia.
- **Auth da API (A-2)** — nenhuma rota tem autenticação; decisão para a testnet.
- **Governança da Gov Reserve** — quórum (51 vs 67), teto de saída, `min_deposit` (risco de spam de propostas).
- **Bloco 2** — notários t-de-n + `dustReserveAddress` (distribuir o notariado).

---

## 5. Estado atual (resumo)

- **Código:** FASE 8 essencialmente fechada (hard test local Blocos 1–4; A-8 e
  T-GOV-1 fechados). Reset da chain feito (2026-10-07).
- **Minerável E2E:** Viacredi (`code 0`). Caixa: rail **FAIL** (anti-bot). BB:
  pendente de sessão. US/CA: falta campo canónico.
- **Falta para abrir:** o checklist da §2.

