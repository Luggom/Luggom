package types

import "testing"

func TestCanonicalUSTaxEntityKey_EINFormats(t *testing.T) {
	if got := CanonicalUSTaxEntityKey("12-3456789"); got != "ein:123456789" {
		t.Fatalf("unexpected key %q", got)
	}
	if got := CanonicalUSTaxEntityKey(" 123456789 "); got != "ein:123456789" {
		t.Fatalf("unexpected key %q", got)
	}
	if got := CanonicalUSTaxEntityKey("IRS"); got != "slug:irs" {
		t.Fatalf("unexpected key %q", got)
	}
}

func TestIsUSTaxEntityWhitelisted_EINEquivalent(t *testing.T) {
	wl := []string{"123456789"}
	if !IsUSTaxEntityWhitelisted("12-3456789", wl) {
		t.Fatal("expected EIN with dashes to match")
	}
	if !IsUSTaxEntityWhitelisted("123456789", wl) {
		t.Fatal("expected raw EIN to match")
	}
	if IsUSTaxEntityWhitelisted("99-9999999", wl) {
		t.Fatal("unexpected match for different EIN")
	}
}
