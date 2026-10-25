#!/usr/bin/env python3
"""Writes every scout SVG from one geometry. Run from anywhere; needs only Python 3,
plus fonttools (`pip install fonttools`) and Inter at /usr/share/fonts/opentype/inter
for the two lockups and the social-preview line (skipped, with a note, when absent).

The mark: an S built from two circles of radius R=300 (centrelines), centres
(512, 212) and (512, 812), drawn as one stroke of width 175 and leaned 8°
clockwise, then cropped by the icon plate (824 of the 1024 canvas, corner
radius 185). The lean puts both exits on the plate's flat edges; the crop is the
design, so the mark is never used without its frame.
"""
import math, os, sys
HERE = os.path.dirname(os.path.abspath(__file__))
R, W, LEAN, T1, T2 = 300, 175, 8, -50, 130
CX = CY = 512
PLATE = '<rect x="100" y="100" width="824" height="824" rx="185" fill="{plate}"/>'
INK, PAPER, PINE, NAVY, CREAM = '#1C1B19', '#F4F1EA', '#17402D', '#122B6E', '#F9F0DF'

def pt(c, t): return (CX + R*math.cos(math.radians(t)), c + R*math.sin(math.radians(t)))
def route():
    x0, y0 = pt(CY-R, T1); x2, y2 = pt(CY+R, T2)
    la1 = 1 if (T1+270) > 180 else 0; la2 = 1 if (T2+90) > 180 else 0
    return f'M{x0:.1f} {y0:.1f} A{R} {R} 0 {la1} 0 {CX} {CY} A{R} {R} 0 {la2} 1 {x2:.1f} {y2:.1f}'
def stroke(colour): return f'<path d="{route()}" fill="none" stroke="{colour}" stroke-width="{W}" transform="rotate({LEAN} {CX} {CY})"/>'

def tile(plate, ink, comment, size=None, full_bleed=False):
    wh = f' width="{size}" height="{size}"' if size else ''
    vb = '100 100 824 824' if full_bleed else '0 0 1024 1024'
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb}"{wh}>\n<!-- {comment} -->\n'
            f'<defs><clipPath id="plate"><rect x="100" y="100" width="824" height="824" rx="185"/></clipPath></defs>\n'
            f'{PLATE.format(plate=plate)}\n<g clip-path="url(#plate)">{stroke(ink)}</g>\n</svg>\n')

def write(name, text):
    with open(os.path.join(HERE, name), 'w') as f: f.write(text)
    print('wrote', name)

write('scout-icon.svg', tile(PINE, PAPER, 'scout app icon: the route in paper on a pine plate. 1024 canvas, 824 plate.', 1024))
write('scout-icon-ink.svg', tile(INK, PAPER, 'scout app icon, mono alternate: paper route on ink.', 1024))
write('scout-icon-navy.svg', tile(NAVY, CREAM, 'scout app icon, family alternate: gnat\'s cream on navy.', 1024))
write('scout-tile-dark.svg', tile(INK, PAPER, 'scout mark for dark surfaces: the tile, full bleed (no canvas margin).', 512, True))
write('scout-tile-light.svg', tile(PAPER, INK, 'scout mark for light surfaces: the tile, full bleed (no canvas margin).', 512, True))
write('scout-favicon.svg', tile(PINE, PAPER, 'scout favicon: the tile, full bleed.', 16, True))
# Icon Composer layer: the route alone on a transparent canvas, at the scale the system's own
# plate expects (the plate is the whole 1024 canvas there), so the 824 plate maps onto 1024.
write('scout-layer.svg', f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="100 100 824 824" width="1024" height="1024">\n'
      f'<!-- Icon Composer layer: route only, transparent; the system draws the plate and masks it. Plate fill: {PINE}. -->\n{stroke(PAPER)}\n</svg>\n')

# Wordmark and tagline outlines.
try:
    from fontTools.ttLib import TTFont
    from fontTools.pens.svgPathPen import SVGPathPen
    from fontTools.pens.transformPen import TransformPen
    def outline(text, font_path, size, tracking_em):
        font = TTFont(font_path); gs = font.getGlyphSet(); cmap = font.getBestCmap()
        scale = size / font['head'].unitsPerEm; x = 0; parts = []
        for ch in text:
            g = gs[cmap[ord(ch)]]; pen = SVGPathPen(gs)
            g.draw(TransformPen(pen, (scale, 0, 0, -scale, x, 0))); parts.append(pen.getCommands())
            x += g.width*scale + tracking_em*size
        return ' '.join(parts), x
    word, ww = outline('scout', '/usr/share/fonts/opentype/inter/Inter-SemiBold.otf', 400, -0.015)
    # Lockup: tile at cap height (Inter cap = 0.727em → 291 for 400), gap of half the tile, equal margins.
    cap = 291; tile_h = cap; gap = cap // 2; m = 60
    for name, plate, ink, text in (('dark', INK, PAPER, PAPER), ('light', PAPER, INK, INK)):
        total_w = m + tile_h + gap + ww + m; total_h = m + cap + m
        write(f'scout-lockup-{name}.svg',
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {total_w:.0f} {total_h}" height="{total_h}">\n'
            f'<!-- scout lockup: tile at cap height + "scout" in Inter SemiBold as outlines. Inter is a placeholder face. -->\n'
            f'<g transform="translate({m} {m}) scale({tile_h/824:.5f}) translate(-100 -100)">'
            f'<defs><clipPath id="lp"><rect x="100" y="100" width="824" height="824" rx="185"/></clipPath></defs>'
            f'{PLATE.format(plate=plate)}<g clip-path="url(#lp)">{stroke(ink)}</g></g>\n'
            f'<g fill="{text}" transform="translate({m+tile_h+gap} {m+cap})"><path d="{word}"/></g>\n</svg>\n')
    tag, tw = outline('Research and plan third-party integrations.', '/usr/share/fonts/opentype/inter/Inter-Medium.otf', 44, 0)
    lw = tile_h + gap + ww; sc = 760 / lw
    write('scout-social.svg',
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 640" width="1280" height="640">\n'
        f'<!-- GitHub social preview: pine field, the lockup, one line saying what scout does. -->\n'
        f'<rect width="1280" height="640" fill="{PINE}"/>\n'
        f'<g transform="translate({(1280-lw*sc)/2:.1f} {320-cap*sc/2-40:.1f}) scale({sc:.4f})">'
        f'<g transform="scale({tile_h/824:.5f}) translate(-100 -100)"><defs><clipPath id="sp"><rect x="100" y="100" width="824" height="824" rx="185"/></clipPath></defs>'
        f'{PLATE.format(plate=PAPER)}<g clip-path="url(#sp)">{stroke(PINE)}</g></g>'
        f'<g fill="{PAPER}" transform="translate({tile_h+gap} {cap})"><path d="{word}"/></g></g>\n'
        f'<g fill="{PAPER}" fill-opacity="0.78" transform="translate({(1280-tw)/2:.1f} 500)"><path d="{tag}"/></g>\n</svg>\n')
except ImportError:
    print('fonttools missing: lockups and social preview not written', file=sys.stderr)
