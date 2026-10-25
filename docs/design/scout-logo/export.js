#!/usr/bin/env node
// Renders the scout PNG package from the SVG sources beside this file, using
// the Chromium that ships with Playwright (`npm i -g playwright` or any local
// install). Run from anywhere: `node docs/design/scout-logo/export.js`.
// favicon.ico is assembled afterwards with ImageMagick: see README.md.
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');
const here = __dirname, out = path.join(here, 'png');
// `color` replaces currentColor: an SVG inside <img> inherits nothing from the page.
const svg = (f, color) => `data:image/svg+xml;base64,${Buffer.from(fs.readFileSync(path.join(here, f), 'utf8').replace(/currentColor/g, color || 'currentColor')).toString('base64')}`;
(async () => {
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch();
  const shot = async (html, w, h, file, transparent) => {
    const page = await browser.newPage({ viewport: { width: w, height: h }, deviceScaleFactor: 1 });
    await page.setContent(`<html><body style="margin:0;${transparent ? 'background:transparent' : ''}">${html}</body></html>`);
    await page.screenshot({ path: path.join(out, file), omitBackground: !!transparent });
    await page.close();
    console.log('wrote png/' + file);
  };
  const img = (f, s, color) => `<img src="${svg(f, color)}" width="${s}" height="${s}" style="display:block">`;
  // App icon at every .iconset size; 16 and 32 use the pixel-grid favicon glyph.
  for (const s of [16, 32, 64, 128, 256, 512, 1024])
    await shot(img(s <= 32 ? 'scout-favicon.svg' : 'scout-icon.svg', s), s, s, `scout-icon-${s}.png`, true);
  // Web: 180 opaque apple-touch-icon (Apple fills transparency with black), 192/512 maskable
  // (the plate already fills the canvas, and the cairn sits inside the inner 80% circle).
  await shot(`<div style="width:180px;height:180px;background:#1C1B19">${img('scout-icon.svg', 180)}</div>`, 180, 180, 'apple-touch-icon.png');
  for (const s of [192, 512])
    await shot(`<div style="width:${s}px;height:${s}px;background:#1C1B19;display:flex;align-items:center;justify-content:center">${img('scout-mark.svg', Math.round(s * 0.56), '#E9A23B')}</div>`, s, s, `maskable-${s}.png`);
  // Mark alone, for READMEs and docs.
  for (const [name, color] of [['charcoal', '#1C1B19'], ['cream', '#F6EEDC'], ['amber', '#E9A23B']])
    await shot(img('scout-mark.svg', 512, color), 512, 512, `scout-mark-${name}-512.png`, true);
  // Lockups.
  for (const v of ['dark', 'light'])
    await shot(`<img src="${svg(`scout-lockup-${v}.svg`)}" height="240" style="display:block">`, Math.round(1870 / 480 * 240), 240, `scout-lockup-${v}.png`, true);
  // GitHub social preview, 1280×640.
  await shot(`<div style="width:1280px;height:640px;background:#1C1B19;display:flex;align-items:center;justify-content:center"><img src="${svg('scout-lockup-dark.svg')}" height="280"></div>`, 1280, 640, 'social-preview.png');
  await browser.close();
})();
