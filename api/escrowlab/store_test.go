package escrowlab

import "testing"

func TestStore_CreateListAccept(t *testing.T) {
	st := NewStore()
	o, err := st.Create(CreateOfferRequest{
		MakerAddress: "luggom1abc",
		GiveDenom:    "uluggom",
		GiveAmount:   "100",
		WantDenom:    "utestatom",
		WantAmount:   "50",
	})
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != "open" {
		t.Fatal(o.Status)
	}
	open := st.ListOpen()
	if len(open) != 1 {
		t.Fatalf("open %d", len(open))
	}
	o2, err := st.Accept(AcceptOfferRequest{OfferID: o.ID, TakerAddress: "luggom1xyz"})
	if err != nil {
		t.Fatal(err)
	}
	if o2.Status != "completed" || o2.TakerAddress != "luggom1xyz" {
		t.Fatalf("%+v", o2)
	}
	if len(st.ListOpen()) != 0 {
		t.Fatal("expected no open")
	}
}

func TestStore_CreateBadDenom(t *testing.T) {
	st := NewStore()
	_, err := st.Create(CreateOfferRequest{
		MakerAddress: "luggom1abc",
		GiveDenom:    "uluggom",
		GiveAmount:   "1",
		WantDenom:    "uatom",
		WantAmount:   "1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStore_Cancel(t *testing.T) {
	st := NewStore()
	o, _ := st.Create(CreateOfferRequest{
		MakerAddress: "luggom1abc",
		GiveDenom:    "uluggom",
		GiveAmount:   "1",
		WantDenom:    "utestusdt",
		WantAmount:   "1",
	})
	_, err := st.Cancel(CancelOfferRequest{OfferID: o.ID, MakerAddress: "luggom1abc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.ListOpen()) != 0 {
		t.Fatal("expected empty")
	}
}
