package podhd500

// Parameter names as shown to the user: whole words, as HX Edit does. The
// catalog keeps the pedal's own names, abbreviated to fit its LCD ("L-Fdbk",
// "ModSpd", "Wow/Flt"…); the API returns these full names.
var fullParamNames = map[string]string{
	// delays
	"Fdbk":      "Feedback",
	"L Time":    "Left Time",
	"R Time":    "Right Time",
	"L-Fdbk":    "Left Feedback",
	"R-Fdbk":    "Right Feedback",
	"ModSpd":    "Mod Speed",
	"ModDep":    "Mod Depth",
	"Wow/Flt":   "Wow/Flutter",
	"Swp Spd":   "Sweep Speed",
	"Swp Dep":   "Sweep Depth",
	"Thresh":    "Threshold",
	"Res":       "Resolution",
	"Heads 1-2": "Heads 1-2",
	"Heads 3-4": "Heads 3-4",
	// modulation
	"VolSens":  "Volume Sensitivity",
	"LFO Shp":  "LFO Shape",
	"Pan Spd":  "Pan Speed",
	"Ch Vib":   "Chorus/Vibrato",
	"Horn Dep": "Horn Depth",
	"Depth2":   "Depth 2",
	"Depth3":   "Depth 3",
	"Regen":    "Regeneration",
	"Even Odd": "Even/Odd",
	"Sw1":      "Switch 1",
	"Sw2":      "Switch 2",
	"Sw3":      "Switch 3",
	"Sw4":      "Switch 4",
	"Freq":     "Frequency",
	// dynamics / distortion
	"Comp": "Compression",
	"Sub":  "Sub Octave",
	// reverb
	"PreDelay": "Predelay",
	// EQ / preamp
	"Low Freq":       "Low Frequency",
	"Hi Freq":        "High Frequency",
	"Hi Gain":        "High Gain",
	"Hi Mid":         "High Mid",
	"Hi":             "High",
	"Hi Pass Freq":   "High Pass Frequency",
	"Hi Pass Q":      "High Pass Q",
	"Low Pass Freq":  "Low Pass Frequency",
	"Hi Pass Filter": "High Pass Filter",
	"Lo Pass Filter": "Low Pass Filter",
	// amp
	"SAG": "Sag",
	"HUM": "Hum",
	// cab
	"Res Level": "Resonance Level",
}

// FullParamName returns the unabbreviated name of a parameter.
func FullParamName(name string) string {
	if f, ok := fullParamNames[name]; ok {
		return f
	}
	return name
}
