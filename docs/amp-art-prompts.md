# Amp pictures: prompts for an image generator

The model grid (block editor, ⊞) shows a large picture of each amp. When
`frontend/public/gear/<file>.png` exists and is listed in
`frontend/public/gear/index.json`, the editor shows that image; otherwise it
shows its own vector drawing.

## Rules (keep every picture consistent and original)

- Generate from **text only**. Never give the generator a screenshot of Helix,
  HX Edit or any Line 6 product, or a photo of a real amp: the result must be
  original art, not a copy.
- **No logos, no brand names, no model names, no lettering** anywhere. A name
  plate, if any, is blank.
- Check that the generator's terms allow public and commercial use of the output
  (this project is MIT-licensed and public).
- Same framing for all: square canvas (1024×1024), the amp centred, filling about
  80% of the width, straight-on front view.

**Style suffix**: add this to the end of every prompt:

> straight-on front view, orthographic, centred, isolated product illustration,
> flat vector style with soft subtle shading, light from the top left, clean
> edges, plain solid #0f0f0f background, soft shadow on the floor, no text, no
> letters, no logos, no brand name, blank name plate, square 1:1 canvas, the
> object filling 80% of the width

## Files

Save each image as PNG with the file name below (for example
`amp-tweed-b-man.png`) in `frontend/public/gear/`, then add the name without
`.png` to `index.json` (or ask Claude to do it).

| File | Prompt (then the style suffix) |
|---|---|
| amp-tweed-b-man | 1950s guitar combo amplifier, wide cabinet covered in light tan tweed cloth with diagonal weave and tan leather piping, dark brown speaker cloth showing the outlines of four 10-inch speakers, controls on the top of the cabinet so none are visible from the front, leather handle on top |
| amp-small-tweed | very small 1950s practice combo amplifier, tan tweed cloth covering with diagonal weave, small dark brown speaker cloth for one 8-inch speaker, controls on top so none are visible from the front, small leather handle |
| amp-blackface-lux | mid-1960s 1x12 guitar combo amplifier, black vinyl covering, silver sparkle grille cloth with a thin silver frame showing one speaker outline, black front control panel along the top with a row of black skirted knobs with silver tops and a red jewel pilot light, black handle |
| amp-blackface-double | large mid-1960s 2x12 guitar combo amplifier, black vinyl covering, wide silver grille cloth showing two speaker outlines, black front control panel along the top with many black skirted knobs and a red jewel light, black handle |
| amp-black-panel-pete | 1960s-style 1x12 guitar combo amplifier, black vinyl covering with white piping, silver grille cloth, black front control panel with silver knobs, boutique look, black handle |
| amp-hiway-100 | early-1970s British 100-watt guitar amplifier head, black vinyl covering, wide flat black front, brushed grey metal control panel along the bottom with a row of black knobs with silver caps, small rocker switches, black handle |
| amp-brit-j-45 | mid-1960s British guitar amplifier head, black vinyl covering with white piping, beige salt-and-pepper fabric front, gold-coloured control panel strip with black knobs, black handle |
| amp-plexi-lead-100 | late-1960s British 100-watt guitar amplifier head, black vinyl covering with white piping, dark basket-weave front cloth, gold plexiglass control panel along the top with a row of gold knobs, two toggle switches |
| amp-variac-ed-plexi | late-1960s British 100-watt guitar amplifier head like a vintage plexi panel head but worn, black vinyl with white piping, light basket-weave front, gold control panel with gold knobs, slightly aged look |
| amp-brit-j-800 | early-1980s British guitar amplifier head, black vinyl covering with white piping, black basket-weave front cloth, gold metal control panel along the top with black knobs and a red power light |
| amp-brit-2204 | early-1980s British single-channel guitar amplifier head, black vinyl covering, dark grille cloth front, brushed gold control panel strip with black knobs, white piping |
| amp-brit-p-75 | early-1970s British guitar amplifier head covered in purple vinyl, silver-grey front panel, row of black knobs, metal corner protectors, black handle |
| amp-class-a-15 | early-1960s British 1x12 guitar combo amplifier, black vinyl covering, brown and tan diamond-pattern grille cloth, narrow copper-coloured panel band at the top with no knobs visible from the front (controls on top), small leather handle |
| amp-class-a-30-tb | 1960s British 2x12 guitar combo amplifier, black vinyl covering, dark diamond-pattern grille cloth showing two speaker outlines, copper-coloured band at the top with no knobs visible from the front (controls on top), leather handle |
| amp-a30-fawn | 1960s British 2x12 guitar combo amplifier covered in light fawn beige vinyl, deep oxblood red diamond-pattern grille cloth, beige panel band at the top with no knobs visible from the front, leather handle |
| amp-super-o | early-1960s American guitar combo amplifier, grey and silver textured vinyl covering, light grey grille cloth with fine lines, controls on top so none are visible from the front, small handle |
| amp-gibtone-185 | late-1930s American guitar combo amplifier, warm tan and golden brown covering, a large round speaker opening in the front showing a dark speaker, small blank metal plate above it, no knobs visible from the front, leather handle |
| amp-divide-9-15 | modern boutique guitar amplifier head, teal blue-green vinyl covering, cream coloured control panel with black chicken-head knobs, rounded corners, retro look |
| amp-jazz-rivet | 1970s solid-state 2x12 guitar combo amplifier, black covering, silver brushed-metal control panel across the top with black knobs, dark grey basket-weave grille showing two large speaker outlines |
| amp-mandarin-80 | 1970s British guitar amplifier head covered in bright orange vinyl, cream white control panel with black knobs and small symbols instead of words, beige basket-weave front band, black handle |
| amp-treadplate | modern high-gain guitar amplifier head, black covering, full front of polished aluminium diamond treadplate metal, black control panel with black knobs, metal corners |
| amp-solo-100 | 1980s American high-gain guitar amplifier head, black covering, mirror chrome front panel with black knobs, red power jewel, chrome corners |
| amp-bomber-uber | modern high-gain guitar amplifier head, black covering, black front with a thin horizontal silver stripe, black knobs, minimal design |
| amp-angel-f-ball | modern German high-gain guitar amplifier head, black covering, black front panel with a red and orange accent band, silver knobs |
| amp-mahadeva | modern boutique guitar amplifier head, black covering, gold brass band across the upper front, rose-beige lower panel with chrome knobs |
| amp-pv-panama | 1990s American high-gain guitar amplifier head, black covering, wide silver brushed-metal control panel with black knobs, black grille band above it |
| amp-phd-motorway | modern boutique guitar amplifier head, dark grey covering, silver brushed-metal control panel with black knobs, simple handle |
| amp-line-6-elektrik | modern high-gain guitar amplifier head, black covering, black front with a bright electric blue control panel, black knobs |
| amp-line-6-doom | modern heavy guitar amplifier head, matte black covering, black front with thin white lines framing the panel, black knobs, dark and minimal |
| amp-line-6-epic | modern guitar amplifier head with a polished silver chrome front, black control panel strip with black knobs, angular modern shape |
| amp-octone | modern guitar amplifier head, dark charcoal covering, black front with teal cyan accent piping, black knobs |
| amp-insane | modern extreme high-gain guitar amplifier head, black covering, black front with red accent piping, black knobs |
| amp-big-bottom | modern guitar amplifier head, dark grey covering, black front with grey accents, large black knobs |
| amp-purge | modern high-gain guitar amplifier head, black covering, black front with green accent piping, black knobs |
| amp-aggro | modern high-gain guitar amplifier head, black covering, black front with violet accent piping, black knobs |
| amp-smash | modern guitar amplifier head, dark brown-black covering, black front with orange accent piping, black knobs with white dots |
| amp-acoustic | acoustic guitar combo amplifier, warm natural wood cabinet, cream control panel with black knobs, brown basket-weave grille cloth |
| amp-flip-top | 1960s bass amplifier combo with a flip-top amplifier head on a cabinet, blue-grey checkered vinyl covering, silver grille, metal clasps |
| amp-svt | 1970s 300-watt bass amplifier head, black covering, silver brushed-metal control panel across the top with black knobs, black ventilated lower front, metal handles on the sides |
| amp-g-cougar-800 | 1990s bass amplifier head in a black metal rack-style case, silver brushed control panel with many small black knobs, slim and wide |
