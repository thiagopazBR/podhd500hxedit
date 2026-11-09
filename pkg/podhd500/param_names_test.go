package podhd500

import (
	"strings"
	"testing"
)

// No LCD abbreviation may reach the UI (checked word by word). Exception:
// "Bias X", kept as HX Edit names it (user, 2026-11-08).
func TestFullParamNames(t *testing.T) {
	bad := map[string]bool{"Fdbk": true, "Spd": true, "Dep": true, "Freq": true, "Thresh": true, "Shp": true,
		"VolSens": true, "Flt": true, "Hi": true, "Lo": true, "Res": true, "Sw1": true, "Sw2": true, "Sw3": true,
		"Sw4": true, "SAG": true, "HUM": true, "ModSpd": true, "ModDep": true, "Regen": true,
		"Comp": true, "PreDelay": true, "Depth2": true, "Depth3": true, "L-Fdbk": true, "R-Fdbk": true, "L": true, "R": true}
	check := func(model, name string) {
		full := FullParamName(name)
		for _, w := range strings.FieldsFunc(full, func(r rune) bool { return r == ' ' || r == '/' }) {
			if bad[w] {
				t.Errorf("%s: %q → %q still abbreviated (%q)", model, name, full, w)
			}
		}
	}
	cat := GetCatalog()
	for _, list := range [][]CatalogModel{cat.Amps, cat.Pedals} {
		for _, m := range list {
			for _, p := range m.Params {
				check(m.Name, p.Name)
			}
		}
	}
	for _, c := range cabParamDefs {
		check("cab", c.Name)
	}
}
