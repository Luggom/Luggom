# Tokenomics — split único (vigente em 2026-11-13)

> Regra canónica das percentagens de cada mineração. O split é **único e
> permanente**; o supply não o altera. A única transição é de **governança**:
> founder-led até 10M LUG minerados; DAO a partir daí.

## Split

| Componente | % |
|---|---:|
| Minerador | 30 |
| **LBC — Luggom Business Core** | **5** (directo, sem cliff) |
| **OTC LOCKED** | **40** |
| Reserva de staking | 7,5 |
| Reserva ATOM | 7,5 |
| Fundador | 5 (cliff de 24 meses) |
| Validadores | 3 |
| Governança (tesouraria) | 1,5 |
| Notariado | 0,5 |
| POOL | — |
| **Total** | **100** |

## Luggom Business Core (LBC)

- **5% de cada mint, directo** para o multisig 3-de-5 (`BusinessCoreAddress`,
  compilado com `-ldflags`).
- **Sem cliff.** O único cliff é o do Fundador (24 meses).
- O endereço é fail-closed: sem `BusinessCoreAddress`, a mineração falha.
- **Sem tecto nem regra especial de voto.** Na DAO, 1 LUG = 1 voto para a LBC
  como para qualquer conta. A Luggom Labs compromete-se publicamente a não
  procurar o controlo da rede — é política declarada, não regra de código.

## OTC LOCKED

- **40% de cada mint**, contínuo e sem tecto, para o cofre do protocolo.
- O cofre vende LUG **só por BTC via IBC** (`ibc/<HASH>`), em lotes mínimos de
  **100.000 LUG**, ao **preço da cesta do dia**.
- O BTC recebido é **lastro permanente**: nunca sai, nunca é queimado, nunca é
  trocado por LUG; só migra de custódia com 87% e para custódia igual ou mais forte.
- **Gate de lotes:** o cofre começa a 0 e recebe 40% de cada mint. Só tem 100k LUG
  depois de **250k LUG minerados** — é a primeira altura em que uma venda OTC é
  possível. Sem pré-mineração para o cofre.
- A compra on-chain (`MsgBuyOtc`) exige o preço **BTC/USD** do dia no oráculo
  (`btc_usd`); sem fixing, falha. O BTC do comprador passa para o cofre e o LUG do
  cofre para a carteira do comprador.
- O denom IBC aceito é **`params.otc_btc_denom`** (default `btc`; na mainnet
  `ibc/<HASH>`). O feeder do oráculo publica o `btc_usd` diário com `--btc-usd`.

## POOL

- **Não há dotação de POOL no protocolo.** A POOL não recebe percentagem no split.
- A LLC pode abrir uma pool como **LP normal** (`AddLiquidity`) quando a governança
  ligar `pool_active`; a liquidez é dela, não do protocolo.
- Sem pool, o OTC é a única via de compra do protocolo. O minerador pequeno não tem
  saída on-chain até a LLC abrir a pool; o OTC exige lotes ≥100k.

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

- **GGOM** paga **200 + 1 LUG** por validador (bond + gás); o bond é
  **empréstimo/seed** ou **self-bond**, não doação.
- Receita do validador: **3% de cada cunhagem** (via `fee_collector` →
  `x/distribution`, por stake) + **0,005 LUG por tx** + **comissão de delegação**.
- **Operadores relacionados:** a Luggom Labs começa com **3 validadores** e o
  fundador com **1** (4 relacionados, a divulgar). Compromisso: 3+1 enquanto o
  conjunto é pequeno; a partir de **40 validadores**, a Labs nunca passa de
  **10% do conjunto**. O único limite on-chain é **1% por validador**, ativo a
  partir de 50 bonded; não existe limite por entidade.

## Governança

- **Antes dos 10M:** founder-led, tally `0 / 10.000 / 0`; só o peso do Fundador conta.
- **A partir dos 10M:** DAO, **1 LUG em staking = 1 voto**, sem multiplicador 33/67.
  LUG fora do staking não vota; delegar dá voto ao validador e o delegador pode
  votar directamente.
- **Quórum 20% · threshold 50% · veto 33%**; **expedited 66,7%** — parâmetros de
  genesis, alteráveis por proposta DAO (não são constantes de código).
- **`min_deposit` 1.000 LUG + 10% inicial**; votação **5 dias**; depósito **3 dias** —
  também parâmetros de genesis.
- **Sem tecto de voto para a LBC** (ver a secção LBC): é um participante, não uma
  classe de voto.
- Tesouraria: tecto **25% em 30 dias**. Notariado: payees publicados, atraso de
  **7 dias**, tecto de **25% do saldo por 7 dias**.
- `x/group` 3-de-5 gera o endereço da LBC.

## Teto, cesta e invariantes

- Teto: **7B LUG no ano 0, −5% ao ano**, janela móvel de 365 dias; máximo vitalício
  teórico ~140B LUG.
- Cesta: `(EUR/USD + GBP/USD + CHF/USD) / 3 × 3`, **+10% ao ano** composto;
  fatores ×1,00 / ×1,61 / ×2,59. A **fórmula** é compilada; os valores do dia vêm
  do oráculo (`usd_oracle_updater_address`). **Não existe preço fixo em USD**: o
  valor em dólar é a fixing do oráculo do dia.
- Guardrails: piso anti-crash **0,75×**; guarda de movimento **10%/dia e 50%**;
  pausa sem fixing em **7 dias**.
- **Sem venda pública/pré-venda.** `MsgBuyPreSale` está desabilitado.
- USDT via **IBC — Kava (primária) / Osmosis (secundária)**, denom real
  `ibc/<HASH>`; **USDC/Noble fora**.
- Taxas: transferência **0,005 LUG**; tentativa de mineração **0,000001 LUG**;
  dust de carteira nova **0,00000000001 LUG** (1× por endereço, máx. 5/hora/IP).
