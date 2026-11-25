package app

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"unicode"
)

// Flavor descriptors are tagged per brew and counted per taster: each user's
// tags across all their included brews of a coffee form one set, so one
// prolific brewer can't push a flavor to the top.

const (
	maxDescriptorsPerBrew = 10
	maxDescriptorLen      = 30
	// flavorMinTasters is how many people must have described a coffee
	// before the community list is shown, so one person's tags don't read as
	// a consensus.
	flavorMinTasters = 3
	// customMinTasters is how many people must use a descriptor that isn't
	// in the catalog before it's shown publicly (keeps typos and junk out).
	customMinTasters = 2
	// flavorTopN caps the list sent to clients.
	flavorTopN = 30
)

type FlavorGroup struct {
	Group string   `json:"group"`
	Items []string `json:"items"`
}

// catalogFlavors follows the SCA/WCR Coffee Taster's Flavor Wheel (2016):
// its top-level groups in wheel order and its everyday terms, plus common
// roaster notes the wheel lacks, filed under the closest group. Lab terms
// (acetic, butyric, isovaleric, citric and malic acid; vanillin, "sweet
// aromatics") are left out. The wheel's papery/musty and chemical terms are
// in a separate "Off-flavors" group at the end, and mouthfeel words, which
// the wheel doesn't cover, get their own group. Entries are lowercase; they
// are also the normalized form stored in the DB.
var catalogFlavors = []FlavorGroup{
	{"Floral", []string{
		"floral", "chamomile", "rose", "jasmine", "hibiscus", "lavender", "orange blossom", "honeysuckle",
		"black tea", "earl grey", "green tea", "oolong",
	}},
	{"Fruity", []string{
		"berry", "blackberry", "raspberry", "blueberry", "strawberry", "cranberry", "black currant",
		"dried fruit", "raisin", "prune", "fig", "date",
		"coconut", "cherry", "pomegranate", "pineapple", "grape", "apple", "pear",
		"stone fruit", "peach", "apricot", "plum", "nectarine",
		"tropical", "mango", "passion fruit", "lychee",
		"citrus", "grapefruit", "orange", "lemon", "lime", "bergamot",
	}},
	{"Sour & fermented", []string{"sour", "winey", "whiskey", "boozy", "fermented", "overripe"}},
	{"Green & vegetative", []string{
		"olive oil", "raw", "under-ripe", "peapod", "fresh", "dark green", "vegetative", "hay-like", "herb-like",
		"herbal", "mint", "tomato", "savory", "beany",
	}},
	{"Roasted", []string{"pipe tobacco", "tobacco", "burnt", "acrid", "ashy", "smoky", "brown roast", "cereal", "grain", "malt", "toast"}},
	{"Spices", []string{"pungent", "pepper", "brown spice", "anise", "nutmeg", "cinnamon", "clove", "cardamom", "ginger"}},
	{"Nutty & cocoa", []string{"nutty", "peanut", "hazelnut", "almond", "walnut", "pecan", "cocoa", "chocolate", "dark chocolate", "milk chocolate"}},
	{"Sweet", []string{"sweet", "brown sugar", "molasses", "maple syrup", "caramel", "honey", "vanilla", "toffee", "butterscotch", "marzipan"}},
	{"Mouthfeel", []string{"juicy", "bright", "clean", "creamy", "buttery", "syrupy", "silky"}},
	{"Off-flavors", []string{
		"papery", "cardboard", "stale", "woody", "musty", "dusty", "earthy", "moldy", "damp", "animalic", "meaty",
		"phenolic", "bitter", "salty", "medicinal", "petroleum", "skunky", "rubber",
	}},
}

var catalogFlavorSet = func() map[string]bool {
	m := map[string]bool{}
	for _, g := range catalogFlavors {
		for _, f := range g.Items {
			m[f] = true
		}
	}
	return m
}()

// normalizeDescriptor lowercases, trims and collapses inner whitespace.
func normalizeDescriptor(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// cleanDescriptors validates and dedupes a brew's descriptors, keeping order.
func cleanDescriptors(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range in {
		d := normalizeDescriptor(raw)
		if d == "" || seen[d] {
			continue
		}
		if len([]rune(d)) > maxDescriptorLen {
			return nil, errf(http.StatusBadRequest, "flavors must be at most %d characters", maxDescriptorLen)
		}
		for _, r := range d {
			if !unicode.IsPrint(r) {
				return nil, errf(http.StatusBadRequest, "flavors can't contain control characters")
			}
		}
		seen[d] = true
		out = append(out, d)
	}
	if len(out) > maxDescriptorsPerBrew {
		return nil, errf(http.StatusBadRequest, "you can tag up to %d flavors per brew", maxDescriptorsPerBrew)
	}
	return out, nil
}

type FlavorCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"` // tasters who used it
	Mine  bool   `json:"mine"`  // the viewer tagged it on one of their brews
}

type Flavors struct {
	Tasters int `json:"tasters"` // people who tagged at least one flavor
	// Top is nil until at least flavorMinTasters people have tagged the
	// coffee (or while the viewer is tasting blind).
	Top []FlavorCount `json:"top"`
}

// flavors returns community descriptor counts for one coffee. Custom
// descriptors need customMinTasters users before they appear.
func (s *statsStore) flavors(coffeeID int64) (tasters int, counts map[string]int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts = map[string]int{}
	a := s.coffees[coffeeID]
	if a == nil {
		return 0, counts
	}
	for _, set := range a.flavors {
		for d := range set {
			counts[d]++
		}
	}
	return len(a.flavors), counts
}

// coffeeFlavors builds the public flavor summary for a coffee. With full set
// to false only the taster count is returned (blind tasting).
func (s *Server) coffeeFlavors(ctx context.Context, viewer *User, coffeeID int64, full bool) (*Flavors, error) {
	tasters, counts := s.stats.flavors(coffeeID)
	f := &Flavors{Tasters: tasters}
	if !full || tasters < flavorMinTasters {
		return f, nil
	}
	mine := map[string]bool{}
	if !viewer.anon {
		rows, err := s.rdb.QueryContext(ctx, `
			SELECT DISTINCT d.descriptor FROM brew_descriptors d JOIN brews b ON b.id = d.brew_id
			WHERE b.user_id = ? AND b.coffee_id = ?`, viewer.ID, coffeeID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var d string
			if err := rows.Scan(&d); err != nil {
				rows.Close()
				return nil, err
			}
			mine[d] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	f.Top = []FlavorCount{}
	for d, n := range counts {
		if n < customMinTasters && !catalogFlavorSet[d] {
			continue
		}
		f.Top = append(f.Top, FlavorCount{Name: d, Count: n, Mine: mine[d]})
	}
	sort.Slice(f.Top, func(i, j int) bool {
		if f.Top[i].Count != f.Top[j].Count {
			return f.Top[i].Count > f.Top[j].Count
		}
		return f.Top[i].Name < f.Top[j].Name
	})
	if len(f.Top) > flavorTopN {
		f.Top = f.Top[:flavorTopN]
	}
	return f, nil
}

// includedFlavors selects brews whose descriptors count toward community
// flavors: the same rule as ratings, but a star rating isn't required.
const includedFlavors = `(b.exclude_from_global = 0)`
