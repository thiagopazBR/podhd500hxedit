package podhd500

import "testing"

func TestSameIgnoringExp(t *testing.T) {
	saved := make([]byte, PresetBodySize)
	q := ItemBaseOffset + 7*ItemSize + ParamBase // item 7, parameter 0
	saved[q+16] = 2                              // on EXP 2
	saved[q+7] = 0x3f                            // value 1.0 (0x3f800000)
	saved[q+6] = 0x80
	buf := append([]byte(nil), saved...)
	buf[q+4], buf[q+5], buf[q+6], buf[q+7] = 0x81, 0x80, 0x80, 0x3c // pedal position ≈0.0157
	if !SameIgnoringExp(saved, buf) {
		t.Fatal("an EXP-driven value counted as an edit")
	}
	buf[q+16] = 0 // controller removed: a real edit
	if SameIgnoringExp(saved, buf) {
		t.Fatal("controller change not seen")
	}
	buf = append([]byte(nil), saved...)
	buf[q+ParamSize+4] = 0x3f // a value without controller
	if SameIgnoringExp(saved, buf) {
		t.Fatal("a real edit was ignored")
	}
}
