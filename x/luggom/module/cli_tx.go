package luggom

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"

	"luggom/x/luggom/types"
)

// GetTxCmd implements autocli.HasCustomTxCommand. Registra comandos manualmente porque
// MsgMineUSD e MsgMineTaxPayment não aparecem no descriptor protobuf fusionado do autocli.
func (AppModule) GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Transactions commands for the luggom module",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(NewMineTaxPaymentCliCmd())
	cmd.AddCommand(NewLockSavingsCliCmd())
	cmd.AddCommand(NewUnlockSavingsCliCmd())
	cmd.AddCommand(NewBuyPreSaleCliCmd())
	cmd.AddCommand(NewDistributePreSaleProceedsCliCmd())
	cmd.AddCommand(NewSpendFromReserveCliCmd())
	cmd.AddCommand(NewUpdateUsdOracleBasketCliCmd())
	return cmd
}

// NewMineTaxPaymentCliCmd returns the cobra command for MsgMineTaxPayment (TLSNotary).
func NewMineTaxPaymentCliCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mine-tax-payment",
		Short: "Mine LUG$ from a TLSNotary tax payment proof (global: EUA + Brasil)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			paymentAmountStr, _ := cmd.Flags().GetString(flagPaymentAmount)
			taxEntity, _ := cmd.Flags().GetString(flagTaxEntity)
			currencyCode, _ := cmd.Flags().GetString(flagCurrencyCode)
			proofCommitment, _ := cmd.Flags().GetString(flagProofCommitment)
			paymentId, _ := cmd.Flags().GetString(flagPaymentID)
			apiAttestation, _ := cmd.Flags().GetString(flagApiAttestation)
			paymentDate, _ := cmd.Flags().GetInt64(flagPaymentDate)
			payerTaxId, _ := cmd.Flags().GetString(flagPayerTaxId)
			taxNatureHash, _ := cmd.Flags().GetString(flagTaxNatureHash)
			providerId, _ := cmd.Flags().GetString(flagProviderID)

			paymentAmount, err := sdkmath.LegacyNewDecFromStr(strings.TrimSpace(paymentAmountStr))
			if err != nil {
				return fmt.Errorf("payment-amount: %w", err)
			}

			fromAddr := clientCtx.GetFromAddress().String()
			msg := &types.MsgMineTaxPayment{
				MinerAddress:    fromAddr,
				PaymentAmount:   paymentAmount,
				TaxEntity:       strings.TrimSpace(taxEntity),
				CurrencyCode:    strings.ToUpper(strings.TrimSpace(currencyCode)),
				ProofCommitment: strings.TrimSpace(proofCommitment),
				PaymentId:       strings.TrimSpace(paymentId),
				ApiAttestation:  strings.TrimSpace(apiAttestation),
				PaymentDate:     paymentDate,
				PayerTaxId:      strings.TrimSpace(payerTaxId),
				TaxNatureHash:   strings.TrimSpace(taxNatureHash),
				// O banco é verificado NA CHAIN contra a whitelist on-chain (§13.131
				// A4). Sem esta flag o comando falhava SEMPRE com 1215 — bug apanhado
				// no review adversarial (§13.146).
				ProviderId: strings.TrimSpace(providerId),
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().String(flagPaymentAmount, "", "eligible payment amount (decimal string)")
	cmd.Flags().String(flagTaxEntity, "", "tax entity slug (irs, ca_ftb, br:receita_federal, ...)")
	cmd.Flags().String(flagCurrencyCode, "USD", "ISO 4217 currency code (USD or BRL)")
	cmd.Flags().String(flagProofCommitment, "", "SHA-256 of TLSNotary proof JSON (anti-double-spend)")
	cmd.Flags().String(flagPaymentID, "", "unique payment transaction ID (anti-replay)")
	cmd.Flags().String(flagApiAttestation, "", "API attestation signature (base64)")
	cmd.Flags().Int64(flagPaymentDate, 0, "Unix timestamp of the payment")
	cmd.Flags().String(flagPayerTaxId, "", "SHA-256 (64 hex) of the payer's EIN/CNPJ/BN — government entities are blocked from mining")
	cmd.Flags().String(flagTaxNatureHash, "", "SHA-256 of the tax-nature fragment extracted by the API (anti-cycle §13.34)")
	cmd.Flags().String(flagProviderID, "", "bank provider id asserted by the notary (e.g. viacredi) — must be in the on-chain whitelist")

	_ = cmd.MarkFlagRequired(flagPaymentAmount)
	_ = cmd.MarkFlagRequired(flagTaxEntity)
	_ = cmd.MarkFlagRequired(flagProofCommitment)
	_ = cmd.MarkFlagRequired(flagPaymentID)
	_ = cmd.MarkFlagRequired(flagApiAttestation)
	_ = cmd.MarkFlagRequired(flagTaxNatureHash)
	_ = cmd.MarkFlagRequired(flagProviderID)

	flags.AddTxFlagsToCmd(cmd)

	return cmd
}

const (
	flagPaymentAmount   = "payment-amount"
	flagTaxEntity       = "tax-entity"
	flagCurrencyCode    = "currency-code"
	flagProofCommitment = "proof-commitment"
	flagPaymentID       = "payment-id"
	flagApiAttestation  = "api-attestation"
	flagPaymentDate     = "payment-date"
	flagPayerTaxId      = "payer-tax-id"
	flagTaxNatureHash   = "tax-nature-hash"
	flagProviderID      = "provider-id"
	// BLOCO D — pre-sale on-chain (§13.132 §4)
	flagUsdtAmount  = "usdt-amount"
	flagDestination = "destination"
	flagAmount      = "amount"
	// Oráculo dinâmico (o feeder usa este comando, 1×/dia — §13.28/§13.56)
	flagChfUsd   = "chf-usd"
	flagEurUsd   = "eur-usd"
	flagGbpUsd   = "gbp-usd"
	flagBrlUsd   = "brl-usd"
	flagCadUsd   = "cad-usd"
	flagRateDate = "rate-date"
	// §14 — reservas (tesouraria e notariado)
	flagReserve = "reserve"
)

// NewUpdateUsdOracleBasketCliCmd returns the cobra command for
// MsgUpdateUsdOracleBasket — a msg que o **feeder do oráculo** submete 1×/dia
// (§13.28/§13.56). Não existia comando CLI para ela; o feeder precisa dele.
func NewUpdateUsdOracleBasketCliCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-usd-oracle-basket",
		Short: "Commit the FX fixing of one day (CHF/EUR/GBP/BRL/CAD) to the dynamic oracle (daily feeder)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			chf, _ := cmd.Flags().GetString(flagChfUsd)
			eur, _ := cmd.Flags().GetString(flagEurUsd)
			gbp, _ := cmd.Flags().GetString(flagGbpUsd)
			brl, _ := cmd.Flags().GetString(flagBrlUsd)
			cad, _ := cmd.Flags().GetString(flagCadUsd)
			rateDate, _ := cmd.Flags().GetString(flagRateDate)

			chfDec, err := sdkmath.LegacyNewDecFromStr(strings.TrimSpace(chf))
			if err != nil {
				return fmt.Errorf("chf-usd: %w", err)
			}
			eurDec, err := sdkmath.LegacyNewDecFromStr(strings.TrimSpace(eur))
			if err != nil {
				return fmt.Errorf("eur-usd: %w", err)
			}
			gbpDec, err := sdkmath.LegacyNewDecFromStr(strings.TrimSpace(gbp))
			if err != nil {
				return fmt.Errorf("gbp-usd: %w", err)
			}
			brlDec, err := sdkmath.LegacyNewDecFromStr(strings.TrimSpace(brl))
			if err != nil {
				return fmt.Errorf("brl-usd: %w", err)
			}
			cadDec, err := sdkmath.LegacyNewDecFromStr(strings.TrimSpace(cad))
			if err != nil {
				return fmt.Errorf("cad-usd: %w", err)
			}

			msg := &types.MsgUpdateUsdOracleBasket{
				Signer:   clientCtx.GetFromAddress().String(),
				ChfUsd:   chfDec,
				EurUsd:   eurDec,
				GbpUsd:   gbpDec,
				BrlUsd:   brlDec,
				CadUsd:   cadDec,
				RateDate: strings.TrimSpace(rateDate),
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String(flagChfUsd, "", "CHF/USD — USD por 1 CHF (ex.: 1.10)")
	cmd.Flags().String(flagEurUsd, "", "EUR/USD — USD por 1 EUR (ex.: 1.18)")
	cmd.Flags().String(flagGbpUsd, "", "GBP/USD — USD por 1 GBP (ex.: 1.37)")
	cmd.Flags().String(flagBrlUsd, "", "BRL/USD — BRL por 1 USD (ex.: 5.04)")
	cmd.Flags().String(flagCadUsd, "", "CAD/USD — CAD por 1 USD (ex.: 1.40)")
	cmd.Flags().String(flagRateDate, "", "fixing date, YYYY-MM-DD (dia UTC da publicação)")
	_ = cmd.MarkFlagRequired(flagChfUsd)
	_ = cmd.MarkFlagRequired(flagEurUsd)
	_ = cmd.MarkFlagRequired(flagGbpUsd)
	_ = cmd.MarkFlagRequired(flagBrlUsd)
	_ = cmd.MarkFlagRequired(flagCadUsd)
	_ = cmd.MarkFlagRequired(flagRateDate)
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// NewBuyPreSaleCliCmd returns the cobra command for MsgBuyPreSale (Bloco D).
// Compra ao preço FIXO de $0,10/LUG (constante compilada), com vesting linear de
// 3 meses; o LUG só chega à carteira à medida que veste.
func NewBuyPreSaleCliCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "buy-pre-sale",
		Short: "Buy LUG from the on-chain pre-sale with USDT ($0,10/LUG, 3-month linear vesting)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			usdtStr, _ := cmd.Flags().GetString(flagUsdtAmount)
			usdt, ok := sdkmath.NewIntFromString(strings.TrimSpace(usdtStr))
			if !ok {
				return fmt.Errorf("usdt-amount: %q não é um inteiro (unidades base, 6 casas)", usdtStr)
			}
			msg := &types.MsgBuyPreSale{
				Buyer:      clientCtx.GetFromAddress().String(),
				UsdtAmount: usdt,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String(flagUsdtAmount, "", "USDT pago, em unidades base (6 casas) — ex.: 1000000000 = 1000 USDT")
	_ = cmd.MarkFlagRequired(flagUsdtAmount)
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// NewDistributePreSaleProceedsCliCmd returns the cobra command for
// MsgDistributePreSaleProceeds (authority-gated): move o USDT acumulado no
// pre-sale para a pool/marketing. Sem ele o dinheiro fica preso na conta de
// módulo (§13.135).
func NewDistributePreSaleProceedsCliCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "distribute-pre-sale-proceeds",
		Short: "Move USDT from the pre-sale to a destination (module authority only)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			dest, _ := cmd.Flags().GetString(flagDestination)
			amountStr, _ := cmd.Flags().GetString(flagAmount)
			amount, ok := sdkmath.NewIntFromString(strings.TrimSpace(amountStr))
			if !ok {
				return fmt.Errorf("amount: %q não é um inteiro (unidades base, 6 casas)", amountStr)
			}
			msg := &types.MsgDistributePreSaleProceeds{
				Authority:   clientCtx.GetFromAddress().String(),
				Destination: strings.TrimSpace(dest),
				Amount:      amount,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String(flagDestination, "", "endereço de destino (ex.: a tesouraria da pool)")
	cmd.Flags().String(flagAmount, "", "montante de USDT em unidades base (6 casas)")
	_ = cmd.MarkFlagRequired(flagDestination)
	_ = cmd.MarkFlagRequired(flagAmount)
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// NewSpendFromReserveCliCmd returns the cobra command for MsgSpendFromReserve.
// Só a authority (o x/gov) consegue assinar: na prática é submetida por uma
// proposta de governança, não pela CLI de um utilizador.
func NewSpendFromReserveCliCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spend-from-reserve",
		Short: "Spend from the treasury (gov) or the notary reserve (gov authority only)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			reserve, _ := cmd.Flags().GetString(flagReserve)
			destination, _ := cmd.Flags().GetString(flagDestination)
			amountStr, _ := cmd.Flags().GetString(flagAmount)

			amount, ok := sdkmath.NewIntFromString(strings.TrimSpace(amountStr))
			if !ok {
				return fmt.Errorf("amount must be an integer in uluggom (18 decimals), got %q", amountStr)
			}

			msg := &types.MsgSpendFromReserve{
				Authority:   clientCtx.GetFromAddress().String(),
				Reserve:     strings.TrimSpace(reserve),
				Destination: strings.TrimSpace(destination),
				Amount:      amount,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String(flagReserve, "", "reserva a gastar: \"gov\" (tesouraria) ou \"notary\" (notariado)")
	cmd.Flags().String(flagDestination, "", "endereço destino (luggom1…)")
	cmd.Flags().String(flagAmount, "", "montante em uluggom (inteiro, 18 casas)")
	_ = cmd.MarkFlagRequired(flagReserve)
	_ = cmd.MarkFlagRequired(flagDestination)
	_ = cmd.MarkFlagRequired(flagAmount)
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
