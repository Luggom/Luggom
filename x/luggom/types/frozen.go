package types

import sdkmath "cosmossdk.io/math"

// §13.170 Grupo B — valores CONGELADOS em código.
//
// Deixaram de ser params: uma proposta de governança já não os alcança. Era este
// o furo que o T-GOV-1 provou ao vivo (o filtro estrito não protege params).
// O Grupo B é o ganho de segurança maior da classificação A–E.
const (
	// TlsnotaryPhaseFrozen: a chain corre SEMPRE em fase 2 (atestação ECDSA
	// assinada pelo notário, verificada contra api_verifier_address). A fase 1
	// (trust-API, forjável) DEIXOU de existir.
	TlsnotaryPhaseFrozen uint32 = 2

	// DynamicUsdOracleEnabledFrozen: o oráculo dinâmico está sempre ligado.
	DynamicUsdOracleEnabledFrozen = true

	// UsdOracleMaxStaleSecondsFrozen: 36 h — 1 snapshot/dia + folga (§13.56).
	UsdOracleMaxStaleSecondsFrozen uint64 = 129_600

	// TaxPaymentMaxAgeDaysFrozen: janela de validade do comprovante.
	TaxPaymentMaxAgeDaysFrozen uint32 = 365
)

// GovernanceQuorumFrozen / VetoThresholdFrozen — limiares do módulo, congelados.
// (O x/gov da chain tem os seus próprios params no genesis; estes são os que o
// govtally do x/luggom consulta.)
var (
	GovernanceQuorumFrozen = sdkmath.LegacyMustNewDecFromStr("0.20")
	VetoThresholdFrozen    = sdkmath.LegacyMustNewDecFromStr("0.33")
)
