package types

// BusinessCoreAddress is the Luggom Business Core wallet — the x/group policy
// address that receives 5% of every mint. Phase 1 credits are retained with a
// frozen clock and released with the Phase 2 credits 24 months after the first
// Phase 2 mint.
//
// It is a COMPILED constant, set at build time with:
//
//	-ldflags "-X luggom/x/luggom/types.BusinessCoreAddress=luggom1..."
//
// Changing it therefore requires a binary upgrade, never a governance vote.
//
// Empty is fail-closed: the split refuses to mint the Business Core slice.
var BusinessCoreAddress = ""
