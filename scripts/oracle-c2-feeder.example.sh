#!/usr/bin/env bash
# Exemplo: obter CHF/USD, EUR/USD, GBP/USD a partir de Frankfurter (base USD)
# e preparar valores para MsgUpdateUsdOracleBasket (ajustar para o teu CLI / cosmovisor).
#
# Frankfurter devolve "rates" como: 1 USD = X CHF  →  CHF/USD = 1/X
# Documentação: https://www.frankfurter.app/docs/
#
# Requisitos: curl, jq
# Uso: export CHAIN_HOME RPC KEYRING ORACLE_KEY; ./scripts/oracle-c2-feeder.example.sh

set -euo pipefail

JSON="$(curl -fsS "https://api.frankfurter.app/latest?from=USD&to=CHF,EUR,GBP")"
CHF_PER_USD="$(echo "$JSON" | jq -r '.rates.CHF')"
EUR_PER_USD="$(echo "$JSON" | jq -r '.rates.EUR')"
GBP_PER_USD="$(echo "$JSON" | jq -r '.rates.GBP')"

# BTC/USD para a venda OTC (Binance; trocar por outra fonte se necessário).
BTC_USD="$(curl -fsS "https://api.binance.com/api/v3/ticker/price?symbol=BTCUSDT" | jq -r '.price')"

# 1 CHF = (1/CHF_PER_USD) USD
CHF_USD="$(echo "scale=18; 1 / $CHF_PER_USD" | bc)"
EUR_USD="$(echo "scale=18; 1 / $EUR_PER_USD" | bc)"
GBP_USD="$(echo "scale=18; 1 / $GBP_PER_USD" | bc)"

echo "chf_usd=$CHF_USD eur_usd=$EUR_USD gbp_usd=$GBP_USD btc_usd=$BTC_USD"
echo "# Exemplo (comando fictício — substituir pelo teu binário e flags reais):"
echo "# luggomd tx luggom update-usd-oracle-basket \\"
echo "#   --chf-usd \"$CHF_USD\" --eur-usd \"$EUR_USD\" --gbp-usd \"$GBP_USD\" \\"
echo "#   --btc-usd \"$BTC_USD\" --rate-date \"$TODAY\" \\"
echo "#   --from \$ORACLE_KEY --chain-id \$CHAIN_ID --node \$RPC -y"
