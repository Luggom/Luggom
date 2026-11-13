# Tokenomics — Tabela C (vigente em 2026-11-13)

> Regra canónica das percentagens de cada mineração. A **Fase 2 começa por supply**
> (≥10M LUG minerados), não por `pool_active`; a pool é um interruptor separado.

## Fases

| Componente | Fase 1 (<10M minerados) | Fase 2 (≥10M) |
|---|---:|---:|
| Minerador | 30 | 30 |
| **LBC — Luggom Business Core** | **5** (retido) | **5** (cliff 24m) |
| **OTC LOCKED** | **30** | **35** |
| Reserva de staking | 7,5 | 10 |
| Reserva ATOM | 7,5 | 10 |
| Fundador | 5 | 5 |
| Validadores | 3 | 3 |
| Governança (tesouraria) | 2 | 1,5 |
| Notariado | — | 0,5 |
| POOL | 10 | — |
| **Total** | **100** | **100** |

## Luggom Business Core (LBC)

- **5% em ambas as fases.** Na Fase 1 a fatia fica **retida** em
  `luggom_business_core_vesting` com o relógio **congelado**.
- O cliff de **24 meses** começa no **primeiro mint da Fase 2**. Até lá, a fatia da
  Fase 2 também fica retida.
- Ao fim do cliff é libertado **todo o acumulado (Fase 1 + Fase 2)** para o multisig
  3-de-5; daí em diante os 5% fluem diretamente.
- O `BusinessCoreAddress` é compilado no binário e é **fail-closed**: sem endereço,
  a libertação falha. Na Fase 1 o endereço não é necessário porque a fatia é retida.

## Pool por necessidade

- A dotação **POOL de 1M LUG** (10% da Fase 1) fica **inteiramente travada** em
  `luggom_pool_treasury`. **Não existe** o par 100k na AMM + 900k.
- A pool **não nasce no genesis**. Abre quando o **lado estável** chega ao
  **preço da fórmula do dia**, de uma de três fontes:
  - **A** — a empresa operadora (LLC);
  - **B** — um consórcio de comerciantes/mineradores;
  - **C** — uma âncora externa (fundo/banco).
- O lado estável necessário é **1M × preço da fórmula**; não há valor fixo em USD.
- O **primeiro depósito fixa a razão**. As shares de LP do abridor ficam **travadas**
  pelo prazo do term sheet; o LP do protocolo **não pode ser retirado**.
- `pool_active` só liga **depois** do seed. Antes disso, `AddLiquidity`/`Swap`/
  `RemoveLiquidity` são rejeitados.
- Enquanto a pool não abre, a única saída on-chain é o **OTC LOCKED**: só BTC,
  lote mínimo de 100.000 LUG, ao preço da cesta.

## Validadores

| Item | Testnet | Mainnet |
|---|---:|---:|
| Bond por validador | 200 LUG | 1.000 LUG |
| Gás de onboarding | 1 LUG | 1 LUG |
| Piso | 20 | 20 |
| Alvo | 40 | 40 |
| Teto (`max_validators`) | 180 | 180 |
| Seed de 40 (bond + gás) | 8.040 LUG | 40.040 LUG |
| Genesis total (com margem) | 8.200 LUG | ~40.100 LUG |

- **GGOM** paga **200 + 1 LUG** por validador (bond + gás). O bond é
  **empréstimo/seed** ou **self-bond**, não doação.
- Receita do validador: **3% de cada cunhagem** (via `fee_collector` →
  `x/distribution`, por stake) + **0,005 LUG por tx** + **comissão de delegação**.

## Governança

- **Fase 1 (<10M):** founder-led, tally `0 / 10.000 / 0`.
- **Fase 2 (≥10M):** **1 LUG = 1 voto**, sem multiplicador 33/67.
- **Quórum 20% · threshold 50% · veto 33%**; **expedited 66,7%**.
- **`min_deposit` 1.000 LUG + 10% inicial**; votação **5 dias**; depósito **3 dias**.
- **Teto da LBC: 20%** do voto direto; delegações da LBC são subtraídas dos
  validadores; sem voto direto, o stake da LBC não conta.
- Tesouraria: tecto **25% em 30 dias**. Notariado: payees publicados, atraso de
  **7 dias**, tecto de **25% do saldo por 7 dias**.
- `x/group` 3-de-5 gera o endereço da LBC (multisig da política).

## Teto, cesta e invariantes

- Teto: **7B LUG no ano 0, −5% ao ano**, janela móvel de 365 dias; máximo vitalício
  teórico ~140B LUG.
- Cesta: `(EUR/USD + GBP/USD + CHF/USD) / 3 × 3`, **+10% ao ano** composto;
  fatores ×1,00 / ×1,61 / ×2,59. **Não existe preço fixo em USD**: o valor em dólar
  é a fixing do oráculo do dia.
- Guardrails: piso anti-crash **0,75×**; guarda de movimento **10%/dia e 50%**;
  pausa sem fixing em **7 dias**.
- **Sem venda pública/pré-venda.** `MsgBuyPreSale` está desabilitado.
- USDT via **IBC — Kava (primária) / Osmosis (secundária)**, denom real
  `ibc/<HASH>`; **USDC/Noble fora**.
- Taxas: transferência **0,005 LUG**; tentativa de mineração **0,000001 LUG**;
  dust de carteira nova **0,00000000001 LUG** (1× por endereço, máx. 5/hora/IP).
