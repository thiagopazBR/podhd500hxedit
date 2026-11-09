package podhd500

// shortCategories: the HX Edit-style label under an FX block in the signal
// flow — its category, abbreviated (Dynamics → Dyn, Reverb → Verb…).
var shortCategories = map[string]string{
	"Distortion": "Dist",
	"Dynamics":   "Dyn",
	"Delay":      "Delay",
	"Modulation": "Mod",
	"Reverb":     "Verb",
	"Pitch":      "Pitch",
	"Filter":     "Filter",
	"Wah":        "Wah",
	"FX Loop":    "FX Loop",
}

// ShortCategory returns the label under an FX block. The HD500's combined
// categories show only the half the model belongs to: Preamp+EQ is "Preamp"
// for Vintage Pre and "EQ" for the EQs; Volume/Pan is "Vol" or "Pan".
func ShortCategory(group, model string) string {
	switch group {
	case "Preamp+EQ":
		if model == "Vintage Pre" {
			return "Preamp"
		}
		return "EQ"
	case "Volume/Pan":
		if model == "Pan" {
			return "Pan"
		}
		return "Vol"
	}
	return shortCategories[group]
}

func shortOf(kind, group, model string, empty bool) string {
	if kind != "fx" || empty {
		return ""
	}
	return ShortCategory(group, model)
}

// AssignLabel is the line under a block naming its pedal-board controls:
// the footswitch that turns it on/off (FS1–FS8, or the EXP pedal's toe
// switch) and the expression pedals driving its parameters (EXP 1 / EXP 2).
// e.g. "FS3", "EXP 2", "EXP 1 · Toe", "FS1 · EXP 1+2".
func AssignLabel(fs uint8, params []ItemParam) string {
	e1, e2 := false, false
	for _, p := range params {
		switch p.Ctl {
		case 1:
			e1 = true
		case 2:
			e2 = true
		}
	}
	exp := ""
	switch {
	case e1 && e2:
		exp = "EXP 1+2"
	case e1:
		exp = "EXP 1"
	case e2:
		exp = "EXP 2"
	}
	sw := ""
	switch {
	case fs >= 1 && fs <= 8:
		sw = FSNames[fs]
	case fs == 9:
		sw = "Toe"
		if exp == "" {
			sw = "EXP Toe"
		}
	}
	switch {
	case exp == "":
		return sw
	case sw == "":
		return exp
	case fs == 9:
		return exp + " · " + sw // "EXP 1 · Toe": the pedal, then its toe switch
	}
	return sw + " · " + exp
}
