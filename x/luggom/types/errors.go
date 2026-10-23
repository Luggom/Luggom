package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/luggom module sentinel errors
var (
	ErrInvalidSigner              = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrInvalidTaxEntity           = errors.Register(ModuleName, 1200, "tax entity not in whitelist")
	ErrInvalidCurrencyCode        = errors.Register(ModuleName, 1201, "unsupported currency code")
	ErrProofAlreadyUsed           = errors.Register(ModuleName, 1202, "proof commitment already used (double-spend)")
	ErrPaymentIDAlreadyUsed       = errors.Register(ModuleName, 1203, "payment id already used (replay)")
	ErrInvalidAttestation         = errors.Register(ModuleName, 1204, "API attestation invalid")
	ErrStalePayment               = errors.Register(ModuleName, 1205, "payment too old (exceeds max age)")
	ErrGovernmentEntityCannotMine = errors.Register(ModuleName, 1206, "government entities cannot mine LUG (EIN/CNPJ-to-EIN/CNPJ transfer)")
	ErrMissingTaxNature           = errors.Register(ModuleName, 1207, "tax nature hash missing or invalid (payment must be a real tax collection)")
	ErrOtcLotBelowMinimum         = errors.Register(ModuleName, 1208, "OTC vault sale lot is below the 10k LUG minimum")
	ErrOtcAssetNotAllowed         = errors.Register(ModuleName, 1209, "OTC vault only sells for BTC")
	ErrOtcIbcNotOpen              = errors.Register(ModuleName, 1210, "OTC vault settlement requires IBC (BTC enters only via IBC)")
	// BLOCO D — pre-sale on-chain (§13.132 §4)
	ErrPreSaleClosed              = errors.Register(ModuleName, 1211, "pre-sale is closed (only Fase 1, before 10M LUG mined)")
	ErrPreSaleBelowMinimum        = errors.Register(ModuleName, 1212, "pre-sale purchase is below the 10 USDT minimum")
	ErrPreSaleCapReached          = errors.Register(ModuleName, 1213, "pre-sale cap reached (3,5M LUG sold)")
	ErrPreSaleNotBacked           = errors.Register(ModuleName, 1214, "pre-sale has not enough LUG in custody to back this purchase")
	ErrBankProviderNotWhitelisted = errors.Register(ModuleName, 1215, "bank provider is not in the on-chain whitelist (§13.131 A4)")
	// §13.146 A — as duas msgs de mineração LEGADAS (zero prova) foram
	// desactivadas: cunhavam só com whitelist de CNPJ + comprimentos mínimos,
	// sem assinatura, sem atestação e ignorando a fase. O caminho com prova é o
	// MsgMineTaxPayment.
	ErrMiningMsgDisabled = errors.Register(ModuleName, 1216, "MsgMine/MsgMineUSD are disabled — use MsgMineTaxPayment (TLSNotary)")
	// §13.174 — a cunhagem é preçada na data do pagamento. Datas futuras não
	// podem passar: com o lookback a andar para trás, uma data alguns dias à
	// frente resolvia para uma fixing passada em vez de falhar fechado.
	ErrFuturePayment = errors.Register(ModuleName, 1217, "payment date is in the future")
	// O snapshot diário é chaveado pela fixing date e é imutável: aceitar uma
	// segunda escrita deixaria uma escrita tardia mudar o preço de um recibo já
	// resolvido.
	ErrOracleDayAlreadyWritten = errors.Register(ModuleName, 1218, "fixing for that day is already written (oracle days are immutable)")
)
