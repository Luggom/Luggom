package types

import "cosmossdk.io/errors"

// Luggom fork errors (FASE 3 — validator stake limit §13.24/§13.26).
// Codes start at 500 to avoid colliding with the upstream sentinel errors (2-45).
var (
	ErrDelegationExceedsStakeLimit = errors.Register(ModuleName, 500, "delegation would exceed the per-validator stake limit (1% of total bonded when ≥50 validators are bonded)")
)
