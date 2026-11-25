package app

import "net/http"

// Catalog entries are suggestions only; users can always type a custom value.

type CatalogGrinder struct {
	Model        string `json:"model"`
	Type         string `json:"type"` // hand, electric
	StockBurrs   string `json:"stockBurrs"`
	BurrSizeMM   int    `json:"burrSizeMm,omitempty"`
	BurrGeometry string `json:"burrGeometry"` // flat, conical
}

var catalogMethods = []string{
	"V60", "Kalita Wave", "Chemex", "Origami", "Orea", "April Brewer", "Fellow Aiden",
	"AeroPress", "Clever Dripper", "Hario Switch", "French Press", "Espresso",
	"Moka Pot", "Batch Brew", "Cold Brew", "Siphon", "Cupping",
}

var catalogGrinders = []CatalogGrinder{
	{"1Zpresso J-Max", "hand", "Stock 48mm conical", 48, "conical"},
	{"1Zpresso JX-Pro", "hand", "Stock 48mm conical", 48, "conical"},
	{"1Zpresso K-Ultra", "hand", "Stock 48mm conical", 48, "conical"},
	{"1Zpresso ZP6 Special", "hand", "Stock 48mm conical", 48, "conical"},
	{"1Zpresso X-Ultra", "hand", "Stock 40mm conical", 40, "conical"},
	{"1Zpresso Q2", "hand", "Stock 38mm conical", 38, "conical"},
	{"Baratza Encore", "electric", "Stock 40mm conical", 40, "conical"},
	{"Baratza Encore ESP", "electric", "Stock 40mm conical", 40, "conical"},
	{"Baratza Virtuoso+", "electric", "Stock 40mm conical", 40, "conical"},
	{"Baratza Sette 270", "electric", "Stock 40mm conical", 40, "conical"},
	{"Baratza Vario", "electric", "Stock 54mm flat", 54, "flat"},
	{"Baratza Forte", "electric", "Stock 54mm flat", 54, "flat"},
	{"Breville Smart Grinder Pro", "electric", "Stock conical", 0, "conical"},
	{"Comandante C40", "hand", "Nitro Blade", 39, "conical"},
	{"DF54", "electric", "Stock 54mm flat", 54, "flat"},
	{"DF64 Gen 2", "electric", "Stock 64mm flat", 64, "flat"},
	{"DF83V", "electric", "Stock 83mm flat", 83, "flat"},
	{"Eureka Mignon Specialita", "electric", "Stock 55mm flat", 55, "flat"},
	{"Fellow Ode Gen 1", "electric", "Ode Gen 1 stock burrs", 64, "flat"},
	{"Fellow Ode Gen 2", "electric", "Fellow Gen 2 Brew Burrs", 64, "flat"},
	{"Fellow Opus", "electric", "Stock 40mm conical", 40, "conical"},
	{"Hario Skerton", "hand", "Stock ceramic conical", 0, "conical"},
	{"Kafatek Monolith Flat", "electric", "Stock 75mm flat", 75, "flat"},
	{"Kingrinder K6", "hand", "Stock 48mm conical", 48, "conical"},
	{"Lagom Mini", "hand", "Stock conical", 0, "conical"},
	{"Option-O Lagom P64", "electric", "Stock 64mm flat", 64, "flat"},
	{"Option-O Lagom P100", "electric", "Stock 98mm flat", 98, "flat"},
	{"Mahlkönig EK43", "electric", "Stock 98mm flat", 98, "flat"},
	{"Mahlkönig EK43S", "electric", "Stock 98mm flat", 98, "flat"},
	{"Mahlkönig X54", "electric", "Stock 54mm flat", 54, "flat"},
	{"Mazzer Philos", "electric", "Stock 64mm flat", 64, "flat"},
	{"Niche Zero", "electric", "Stock 63mm conical", 63, "conical"},
	{"Niche Duo", "electric", "Stock 83mm flat", 83, "flat"},
	{"OXO Brew Conical", "electric", "Stock conical", 0, "conical"},
	{"Timemore C3", "hand", "Stock 38mm conical", 38, "conical"},
	{"Timemore Chestnut S3", "hand", "Stock 42mm conical", 42, "conical"},
	{"Timemore Sculptor 064S", "electric", "Stock 64mm flat", 64, "flat"},
	{"Timemore Sculptor 078S", "electric", "Stock 78mm flat", 78, "flat"},
	{"Varia VS3", "electric", "Stock 38mm conical", 38, "conical"},
	{"Weber EG-1", "electric", "Stock 80mm flat", 80, "flat"},
	{"Weber Key", "hand", "Stock 83mm conical", 83, "conical"},
	{"Wilfa Uniform", "electric", "Stock 58mm flat", 58, "flat"},
	{"Zerno Z1", "electric", "Stock 64mm flat", 64, "flat"},
}

var catalogBurrsets = []string{
	"SSP 64mm High Uniformity",
	"SSP 64mm Multipurpose",
	"SSP 64mm Unimodal (Lab Sweet)",
	"SSP 64mm Cast",
	"SSP 83mm High Uniformity",
	"SSP 83mm Multipurpose",
	"SSP 98mm High Uniformity",
	"SSP 98mm Multipurpose",
	"SSP 54mm Brew",
	"Fellow Gen 2 Brew Burrs",
	"Mahlkönig 98mm Coffee Burst",
	"Mizen 64mm Omni",
	"Mizen 64mm Espresso",
	"Ditting 64mm Lab Sweet",
	"Shuriken 64mm",
	"Weber EG-1 Core",
	"Weber EG-1 Ultra",
	"Nitro Blade",
	"Stock burrs",
}

func (s *Server) catalog(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, 200, map[string]any{
		"methods":  catalogMethods,
		"grinders": catalogGrinders,
		"burrsets": catalogBurrsets,
		"flavors":  catalogFlavors,
	})
	return nil
}
