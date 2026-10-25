# scout — logo and app icon

Status: **proposal, 2026-10-25**. A mark for `scout`, the tool Craig uses to
research and plan third-party integrations. Everything here was drawn from
the brief below; the exploration that led to it is in `concepts/`.

## The mark

A **cairn**: three stones stacked by a scout to mark the route for the
people who follow. That is what the tool does — it goes ahead into a third
party's docs and API, works out the way through, and leaves a plan the
builders can walk. Each stone tilts a little, so the stack reads as placed by
hand rather than extruded, in the way Claude's uneven burst does.

![scout lockup](png/scout-lockup-light.png)

Why a cairn and not the obvious things:

- **Compass** — Docker Scout already is a compass on a green disc.
- **Magnifying glass, radar, telescope** — read as search, monitoring, and
  astronomy; all crowded.
- **Plug, puzzle piece, braces, node graph** — the generic integration set.
- **A route ending in a ring** (explored at length, `concepts/round-3.png`)
  reads as a key at every size.

The cairn is one colour, one silhouette, and nothing in the dev-tool space
uses it. It survives 16px as three bars, which no other candidate did.

## Palette

Two colours per surface, one brand colour overall.

| Role | Hex | Where |
|---|---|---|
| Amber (brand) | `#E9A23B` | the cairn on the app icon, favicon and lockups |
| Charcoal | `#1C1B19` | icon plate; the mark on light surfaces |
| Cream | `#F6EEDC` | plate of the light alternates; the mark on dark surfaces |

Trail blazes are painted amber for the same reason it works here: it is
rare on screen, warm, and reads against both charcoal and cream. The amber
sits well away from Claude's terracotta and Raycast's red, and is nowhere
near Docker Scout's mint.

Alternates, for a decision, in `concepts/`'s last sheet and as files:

- `scout-icon-light.svg` — charcoal cairn on cream. Quietest.
- `scout-icon-navy.svg` — navy `#1E3A8A` on cream, the gnat family's paper
  and ink. Pick this to make scout read as gnat's sibling.
- `scout-icon-amber.svg` — charcoal cairn on an amber plate. Loudest.

## Files

| File | What it is |
|---|---|
| `scout-mark.svg` | The cairn alone, `fill="currentColor"`, tight square viewBox. Use in READMEs and docs; colour it with CSS. |
| `scout-icon.svg` | App icon master, 1024 canvas, 824 plate, corner radius 185: amber cairn on charcoal. One icon for both appearances, as Raycast ships. |
| `scout-icon-{light,navy,amber}.svg` | The alternates above. |
| `scout-glyph-16.svg` | The cairn redrawn on a 16-pixel grid with 1px gaps. Scaling the master to 16px merges the stones; this does not. |
| `scout-favicon.svg` | The 16px glyph on its plate. |
| `scout-lockup-{dark,light}.svg` | Mark + "scout" in Inter SemiBold, converted to outlines, so no font is needed to render it. |
| `png/scout-icon-{16…1024}.png` | The `.iconset` sizes. 16 and 32 come from the 16px glyph, the rest from the master. |
| `png/favicon.ico` | 16 + 32 + 48. |
| `png/apple-touch-icon.png` | 180px, opaque (iOS fills transparency with black). |
| `png/maskable-{192,512}.png` | PWA maskable icons; the cairn sits inside the inner 80% safe circle. |
| `png/scout-mark-{charcoal,cream,amber}-512.png` | The mark alone, transparent. |
| `png/social-preview.png` | 1280×640 for the GitHub repository's social preview. |
| `export.js` | Regenerates every PNG from the SVGs with Playwright's Chromium. |
| `concepts/round-*.png` | The five exploration sheets, in order. |

### macOS

For a classic bundle, feed `png/scout-icon-*.png` to `iconutil` as an
iconset, the way `macos/Scripts/make-icon.sh` does for gnat. For macOS 26,
open `scout-icon.svg`'s cairn (the `<g>`; drop the plate rect) in Icon
Composer as the one layer: the system draws plate, rim and shadow; set the
default and dark fill to charcoal, and the glyph is already a single colour
so the tinted and clear appearances need nothing more.

### Regenerating

```sh
node docs/design/scout-logo/export.js
cd docs/design/scout-logo/png && convert scout-icon-16.png scout-icon-32.png \( scout-icon-64.png -resize 48x48 \) favicon.ico
```

The lockup's outlines came from Inter SemiBold via fonttools (520 units,
−1% tracking); re-run only if the wordmark changes.

## Brief

The parameters this was drawn to, from the research pass (three agents:
reference-brand visual language, scout metaphors and name collisions,
platform specs). Kept so the next revision argues with the same facts.

**Form.** One simple flat shape, uniform weight, built on a grid with
rounded corners; no texture, bevel, shadow or 3D. Asymmetry is fine. A
gradient is optional at most (Zed sanctions a flat variant; Linear and
Cursor both moved from gradients to flat). OpenAI's redraw made its stroke
uniform. Modern marks are modified only by flattening or rounding, never by
decorating.

**Colour.** Two at most on any surface: one accent plus a neutral. Separate
light and dark artwork; a mono glyph for macOS 26's tinted mode. Reference
palettes: Zed one blue; Claude one terracotta; OpenAI black or white; Cursor
near-black and off-white; Raycast one red on `#151515`.

**Meaning.** Research + plan + connect, told as scouting: go first, look,
mark the way. Shortlist from the metaphor pass: trail blaze, waypoint
route, S-path with nodes, lookout, cairn, bold spyglass.

**Avoid.** Compass (Docker Scout: compass needle on a mint disc), red
square with white chevrons (Laravel Scout), magnifier, radar (Stripe and
Cloudflare Radar), plug, puzzle piece, braces, node graph.

**Technical.** 1024 master; plate 824 with corner radius ≈0.225 of the
plate; mark about 60% of the plate's height; nothing thinner than 1/16 of
the mark, so a 16px render keeps every stroke at a pixel; a separate glyph
on the 16-pixel grid rather than a scaled master; favicon as SVG + 32 PNG
+ 180 apple-touch + 192/512 maskable (inner 80% circle safe); GitHub
social preview 1280×640; README logo via `<picture>` with a
`prefers-color-scheme: dark` source.

**Rounds.** Pictograms (route, spyglass, lookout, blaze) in rounds 1–3 were
too small and off-centre for the taste set, where every mark is one bold
centred shape, and the route family read as keys. Round 4 moved to bold
centred forms; the cairn won over the periscope (reads as stairs), the
binoculars (reads as an infinity sign) and a fat S (handsome, says nothing).
Round 5 flattened the stones so the stack stops reading as a figure at
16px, widened the gaps so they survive 32px, and tried the palettes.
