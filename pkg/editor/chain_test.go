package editor

import (
	"os"
	"reflect"
	"testing"

	"podhd500-edit/pkg/podhd500"
)

// chainPreset: FX 4..11 in lanes like the user's 2C after the pedal moves:
// pre: 5,4,6  bPre: 8  post: 7,10,11,9.
func chainPreset(t *testing.T) *podhd500.Preset {
	t.Helper()
	blank, err := podhd500.EmbeddedData.ReadFile("data/blank_preset.bin")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := podhd500.ParsePreset(blank)
	set := map[int][2]int{5: {0, 0}, 4: {0, 1}, 6: {0, 2}, 8: {2, 3}, 7: {5, 4}, 10: {5, 5}, 11: {5, 6}, 9: {5, 7}}
	for id, lp := range set {
		o := podhd500.ItemOffset(id)
		p.Body[o+6] = byte(lp[0])
		p.Body[o+4], p.Body[o+5] = byte(lp[1]), 0
	}
	return p
}

func lanes(p *podhd500.Preset) [][]int {
	c := chainOf(p)
	out := make([][]int, NumLanes)
	for i, l := range c.Lanes {
		out[i] = l.Blocks
	}
	return out
}

func TestChainOf(t *testing.T) {
	got := lanes(chainPreset(t))
	want := [][]int{{5, 4, 6}, {}, {8}, {}, {}, {7, 10, 11, 9}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestMoveFX(t *testing.T) {
	cases := []struct {
		name             string
		id, lane, before int
		want             [][]int
	}{
		{"within lane, left", 6, LanePre, 5, [][]int{{6, 5, 4}, {}, {8}, {}, {}, {7, 10, 11, 9}}},
		{"within lane, to end", 5, LanePre, -1, [][]int{{4, 6, 5}, {}, {8}, {}, {}, {7, 10, 11, 9}}},
		{"pre to post, before 11", 4, LanePost, 11, [][]int{{5, 6}, {}, {8}, {}, {}, {7, 10, 4, 11, 9}}},
		{"post to empty lane aPost", 9, LaneAPost, -1, [][]int{{5, 4, 6}, {}, {8}, {9}, {}, {7, 10, 11}}},
		{"onto itself", 8, LaneBPre, 8, [][]int{{5, 4, 6}, {}, {8}, {}, {}, {7, 10, 11, 9}}},
	}
	for _, c := range cases {
		p := chainPreset(t)
		if err := moveFX(p, c.id, c.lane, c.before); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := lanes(p); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("%s: got %v want %v", c.name, got, c.want)
		}
		// positions are a permutation of 0..7
		seen := map[int]bool{}
		for _, s := range fxSlots(p) {
			if s.pos < 0 || s.pos > 7 || seen[s.pos] {
				t.Fatalf("%s: bad positions %v", c.name, fxSlots(p))
			}
			seen[s.pos] = true
		}
	}
	p := chainPreset(t)
	for _, bad := range [][3]int{{0, 0, -1}, {4, 6, -1}, {4, LanePost, 5}} { // amp, bad lane, before not in lane
		if err := moveFX(p, bad[0], bad[1], bad[2]); err == nil {
			t.Fatalf("expected error for %v", bad)
		}
	}
}

func TestAmpPos(t *testing.T) {
	for _, c := range []struct {
		code uint16
		ampB bool
		want string
	}{{5, false, "pre"}, {7, false, "post"}, {0, false, "a"}, {5, true, "a"}} {
		p := chainPreset(t)
		o := podhd500.ItemOffset(0)
		p.Body[o+4], p.Body[o+5] = byte(c.code), byte(c.code>>8)
		ob := podhd500.ItemOffset(2)
		if c.ampB { // any real model in amp B
			p.Body[ob], p.Body[ob+1], p.Body[ob+11] = 0x05, 0x00, 1
		} else {
			p.Body[ob], p.Body[ob+1] = 0xff, 0xff
		}
		if got := chainOf(p).AmpPos; got != c.want {
			t.Errorf("code %d ampB %v: got %q, want %q", c.code, c.ampB, got, c.want)
		}
	}
}

// Buffers recorded while the user moved the amp on the pedal (User 2 4B):
// amp alone on path B (amp A disabled) → amp A to post → back to the paths.
func TestSetAmpPosMatchesPedal(t *testing.T) {
	read := func(name string) *podhd500.Preset {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		p, err := podhd500.ParsePreset(b)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, c := range []struct{ from, pos, want string }{
		{"amp_on_b.bin", "post", "amp_post.bin"},
		{"amp_post.bin", "a", "amp_back_to_paths.bin"},
		{"amp_paths_disabled.bin", "pre", "amp_pre_disabled.bin"},
	} {
		p, want := read(c.from), read(c.want)
		changed, err := setAmpPos(p, c.pos)
		if err != nil || !changed {
			t.Fatalf("%s → %s: changed %v, err %v", c.from, c.pos, changed, err)
		}
		if p.Body != want.Body {
			for i := range p.Body {
				if p.Body[i] != want.Body[i] {
					t.Errorf("%s → %s: byte %#x = %#x, pedal wrote %#x", c.from, c.pos, i, p.Body[i], want.Body[i])
				}
			}
		}
		if got := chainOf(p).AmpPos; got != c.pos {
			t.Errorf("ampPos %q", got)
		}
	}
	if _, err := setAmpPos(read("amp_post.bin"), "b"); err == nil {
		t.Error("amp B position should be refused")
	}
}
