package types

import "testing"

func TestResolveTaxEntityByName(t *testing.T) {
	cases := []struct {
		text string
		want string
		ok   bool
	}{
		// Canadá — cidade pelo nome (comprovante não traz o BN do município).
		{"City of Vancouver", "ca:city:vancouver", true},
		{"CITY OF TORONTO - Property Tax", "ca:city:toronto", true},
		{"City of Abbotsford", "ca:city:abbotsford", true},
		{"Ville de Montreal", "ca:city:montreal", true},
		// Canadá — província e CRA.
		{"Province of British Columbia", "ca:bc", true},
		{"Canada Revenue Agency - GST/HST", "ca:cra", true},
		// EUA — condado pelo nome (nome + UF compõem o slug).
		{"Yavapai County Treasurer", "us:county:yavapai_az", true},
		{"Los Angeles County Tax Collector", "us:county:los_angeles_ca", true},
		// Ambíguo (Orange existe na CA, FL e NY) → NÃO resolve.
		{"Orange County Treasurer", "", false},
		// Estado DEPOIS do nome ("County, ST") desfaz a ambiguidade — o código
		// tem de estar em MAIÚSCULAS no comprovante.
		{"Orange County, CA", "us:county:orange_ca", true},
		{"Orange County, FL Treasurer", "us:county:orange_fl", true},
		{"Washington County, OR", "us:county:washington_or", true},
		{"Yavapai County, AZ", "us:county:yavapai_az", true},
		// Minúsculas não contam (senão "in"/"or"/"me" casariam palavras comuns).
		{"Orange County, ca", "", false},
		// County único na whitelist mas o documento diz OUTRO estado → não
		// aceitar (não creditar o único candidato só por ser único).
		{"Yavapai County, CA", "", false},
		// Concessionária/serviço com nome "quase-fiscal" → NÃO resolve.
		{"Yavapai County Water District", "", false},
		{"City of Vancouver Utilities", "", false},
		// Nada a ver → NÃO resolve.
		{"Acme Comercio Ltda", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := ResolveTaxEntityByName(c.text)
		if ok != c.ok || got != c.want {
			t.Errorf("ResolveTaxEntityByName(%q) = (%q, %v); quer (%q, %v)", c.text, got, ok, c.want, c.ok)
		}
	}
}
