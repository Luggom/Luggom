package types

// DefaultGenesis returns the default genesis state
func DefaultGenesis() *GenesisState {
	params := DefaultParams()

	// Note: fundador_address should be set from config.yml during genesis initialization
	// For MVP, it will be the address of the "smurf" account from config.yml
	// This is handled in the genesis initialization process

	return &GenesisState{
		Params: params,
	}
}

// ValidateGenesisParams validates genesis params, ensuring fundador_address is set if needed.
//
// §13.170: quorum/veto deixaram de ser params do módulo (Grupo B) — passaram a
// constantes de código (frozen.go); a validação desses limiares vive lá.
func ValidateGenesisParams(params Params) error {
	if params.FounderAddress != "" {
		if err := params.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	return gs.Params.Validate()
}
