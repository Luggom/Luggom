package types

import "testing"

func TestValidateMsgMineUSDAnchors(t *testing.T) {
	err := ValidateMsgMineUSDAnchors(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"proof12345",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateMsgMineUSDAnchors("short", "proof12345", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); err == nil {
		t.Fatal("expected error for short document_hash")
	}
}
