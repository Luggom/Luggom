# Upgrades — altura de halt

> **Porquê isto existe.** Enquanto o conjunto de validadores é uma máquina só, um
> upgrade é "parar os quatro nós e trocar o binário". Quando entram validadores
> externos, isso deixa de funcionar: cada nó tem de parar **na mesma altura** e
> voltar com o **mesmo binário**, ou os que ficam para trás divergem e são
> *jailed*.

## Como se anuncia um upgrade

O operador publica, no grupo do Telegram e na release:

- a **altura de halt** (não uma hora — a altura é o que o consenso partilha);
- o **sha256** do novo `luggomd`;
- o que mudou e se o layout de estado mudou;
- o prazo (pelo menos 24 h de aviso).

Regra prática: 5 s por bloco, portanto 24 h são cerca de **17.280 blocos**. A
altura anunciada é `altura atual + (horas × 720)`.

> O `x/upgrade` está ligado na app mas **não tem handler registado**. Um
> `SoftwareUpgrade` por governança não faz o que se espera. Até isso mudar, o
> mecanismo é `--halt-height`.

## O que cada validador faz

### 1. Dias antes — verifica, não troques

Descarrega o binário novo ao lado do antigo e confirma:

```sh
sha256sum ./luggomd-new        # tem de coincidir com o hash anunciado
```

Depois verifica o nó e a distância à altura:

```sh
./scripts/upgrade-preflight.sh \
  --bin ./luggomd-new \
  --home /opt/luggom/home \
  --node tcp://127.0.0.1:26657 \
  --sha256 <hash-anunciado> \
  --halt-height <altura-anunciada>
```

Imprime `PRONTO` ou diz o que falta (hash, sincronização, distância).

### 2. Antes da altura — arma o halt

O nó precisa de `--halt-height` e, durante a janela, de `Restart=no`.
**Esta segunda parte importa:** o `luggomd` sai com **status 1** quando faz halt,
por isso uma unit com `Restart=on-failure` (o default do nosso pacote) reinicia-o,
ele volta a fazer halt, e fica num ciclo até a altura passar.

```sh
sudo systemctl edit luggom-chain
```

```ini
[Service]
Restart=no
ExecStart=
ExecStart=/opt/luggom/build/luggomd start --home /opt/luggom/home --halt-height <ALTURA>
```

```sh
sudo systemctl daemon-reload
sudo systemctl restart luggom-chain
```

### 3. Na altura — o nó pára sozinho

Quando a altura é atingida, o nó pára. No log vais ver:

```
ERR error in proxyAppConn.FinalizeBlock err="halt per configuration height N ..."
ERR CONSENSUS FAILURE!!! err="failed to apply block; error halt per configuration height N"
INF Stopping Node
```

**Isto é o halt, não uma falha.** O processo sai com status 1. O bloco da altura
de halt **não** é aplicado — o nó retoma a partir do anterior.

### 4. Troca e arranque

```sh
sudo install -m 0755 ./luggomd-new /opt/luggom/build/luggomd
```

Remove o drop-in (ou pelo menos o `--halt-height` e o `Restart=no` —
**arrancar o binário novo com a flag de halt ligada pára-o novamente de
imediato**):

```sh
sudo systemctl edit --full luggom-chain   # ou: sudo rm /etc/systemd/system/luggom-chain.service.d/override.conf
sudo systemctl daemon-reload
sudo systemctl start luggom-chain
```

### 5. Verificar

- `catching_up: false` e a altura acima da altura de halt;
- `luggomd query staking validators` mostra `BOND_STATUS_BONDED`;
- o nó está a assinar blocos (sem `missed_blocks` a crescer).

## Se perderes a altura

**Não reinicies o binário antigo.** Descarrega o novo e arranca com ele.

Um nó que estava parado durante o halt e volta com o binário antigo falha no
primeiro bloco que não consegue validar, e acumula *downtime* — o que significa
*jail*. O `unjail` é possível (`luggomd tx slashing unjail`), mas o bond fica
preso durante o período de unbonding.

## Rollback

Não há rollback de estado depois do halt: a chain avançou. Guarda o binário
antigo apenas para o caso de o **novo** não arrancar — e mesmo aí, restaurar
implica repor o diretório de dados a partir de um backup anterior ao halt.

## Lista de verificação

- [ ] hash anunciado confirmado
- [ ] preflight imprimiu `PRONTO`
- [ ] `--halt-height` e `Restart=no` no drop-in
- [ ] nó parou na altura (status 1 é o esperado)
- [ ] binário trocado, flags de halt removidas
- [ ] `catching_up: false`, altura acima do halt, `BONDED`, a assinar

## Nos nossos nós (o operador)

Os quatro nós correm a partir de **uma** unit que executa o
`scripts/run-local-testnet.sh`, por isso o halt arma-se com uma variável de
ambiente, não editando a linha de arranque:

```sh
sudo systemctl edit luggom-chain
```

```ini
[Service]
Environment=LUGGOM_HALT_HEIGHT=<ALTURA>
Restart=no
```

```sh
sudo systemctl daemon-reload && sudo systemctl restart luggom-chain
ps -o args= -p "$(cat /opt/luggom/home/testnet-pids/fundador.pid)" | grep -o -- '--halt-height [0-9]*'
```

A última linha tem de imprimir a flag — se não imprimir, a variável não chegou ao
runner e os nós **não** param.

Depois da troca: põe `LUGGOM_HALT_HEIGHT=0` (ou remove o drop-in), tira o
`Restart=no`, e arranca outra vez.

**Teste de fumo, sem upgrade nenhum:** arma uma altura longe (atual + 50.000),
reinicia, confirma a flag no `ps`, volta a pôr `0` e reinicia. Nada pára, e a
canalização fica provada.
