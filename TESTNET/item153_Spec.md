# Item 153 — Domínio do banco on-chain (self-service)

> Objetivo: adicionar banco = governança on-chain. Depois de **UMA** re-assinatura do
> `.xpi`, nunca mais re-assinar/rebuildar para incluir um banco.

## Estado atual (3 sítios hardcoded)

1. `x/luggom/types/operational_lists.go` → `DefaultBankProviders()` = `[]string` (só IDs/slugs).
2. `api/reclaimverify/providers.go` → `KnownProviders()` = mapa slug→domínio, hardcoded em Go.
3. `coletor-browser/manifest.json` → `host_permissions` = só Caixa/Viacredi/BB.

## A. Proto (domínio vira dado on-chain)

- `proto/luggom/luggom/v1/tx.proto` (linhas 344-350): `MsgAddBankProvider` ganha `string domain` (e opcional `name`, `country`).
- `proto/luggom/luggom/v1/params.proto`: `bank_providers` deixa de ser `repeated string` e vira
  `repeated BankProvider { string provider_id; string domain; string name; string country; }`.
- Regenerar: `./scripts/regenerate_protos.sh` (protoc 25.3).

## B. Chain (keeper + types)

- `x/luggom/types/operational_lists.go`: `DefaultBankProviders()` retorna a lista estruturada
  (espelha o que hoje está em `KnownProviders()`).
- `x/luggom/keeper/msg_operational_lists.go` (`AddBankProvider`): validar `domain` não-vazio
  (fail-closed) e gravar id+domain. `RemoveBankProvider` segue igual.

## C. API + extrator (ler on-chain)

- `api/reclaimverify/providers.go`: `KnownProviders()` deixa de ser a fonte; a resolução de
  domínio passa a ler a lista on-chain (LCD REST `:1317`) com cache curto.
- `cmd/luggom-extract/main.go` (`resolveProvider`): usa a lista on-chain em vez de `KnownProviders()`.
- ⚠️ **Decisão de design:** o extrator hoje é um binário puro (não fala com a chain). Vai precisar
  de um cliente de chain (query REST) ou receber a lista do notário/API como input.
  Recomendado: o extrator consulta a LCD com cache.

## D. Coletor (whitelist dinâmica)

- `coletor-browser/manifest.json`: `host_permissions` → `["<all_urls>"]`; `content_scripts.matches` → `<all_urls>`.
- `coletor-browser/background.js`: buscar a whitelist on-chain (via API) e **só** coletar/enviar
  para domínios que estejam nela.
- `coletor-browser/content.js`: o hook já é genérico; confirmar a injeção dinâmica
  (`scripting.executeScript` na whitelist).

## E. Segurança (importante)

- O **domínio passa a ser governável** ⇒ a entrada de um banco tem de ser **por evidência**
  (recibo real sanitizado + campo canónico + extrator validado), não por voto solto.
  Isso deve virar regra no fluxo de governança.
- `<all_urls>` **atrai escrutínio na AMO** — documentar a justificativa no submit
  (é por isso que a re-assinatura é feita **uma vez**, conscientemente).

## Sequência sugerida

```
A (proto + regen) → B (chain) → C (API/extrator) → D (coletor, 1 re-assinatura) → testes + evidência
```

## Resultado esperado

Depois do **D**: adicionar um banco dos EUA/Canadá = devs entregam evidência → governança aprova
(`AddBankProvider` com id+domínio) → o domínio entra on-chain → o coletor (já com `<all_urls>`) e
o extrator passam a aceitar. **Zero re-assinatura, zero rebuild, zero dependência do Fundador.**
