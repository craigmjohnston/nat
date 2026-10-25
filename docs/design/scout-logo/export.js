#!/usr/bin/env node
// Renders every PNG in png/ from the SVGs beside this file with Playwright's Chromium.
// Needs `playwright` resolvable: run from a directory that has it installed, or set
// NODE_PATH to a global node_modules. `node export.js` writes png/ and lists each file;
// favicon.ico is then assembled with ImageMagick (see README).
const { chromium } = require('playwright');
const fs = require('fs'); const path = require('path');
const here = __dirname, out = path.join(here, 'png');
const uri = (f) => `data:image/svg+xml;base64,${fs.readFileSync(path.join(here, f)).toString('base64')}`;
(async () => {
  fs.mkdirSync(path.join(out, 'scout.iconset'), { recursive: true });
  const browser = await chromium.launch();
  const shot = async (file, w, h, html, transparent) => {
    const page = await browser.newPage({ viewport: { width: w, height: h }, deviceScaleFactor: 1 });
    await page.setContent(`<html><body style="margin:0">${html}</body></html>`);
    await page.screenshot({ path: path.join(out, file), omitBackground: !!transparent });
    await page.close(); console.log('wrote png/' + file);
  };
  const img = (f, s) => `<img src="${uri(f)}" width="${s}" height="${s}" style="display:block">`;
  // macOS .iconset: the inset-plate icon at every size, named as iconutil expects.
  for (const s of [16, 32, 128, 256, 512]) {
    await shot(`scout.iconset/icon_${s}x${s}.png`, s, s, img('scout-icon.svg', s), true);
    await shot(`scout.iconset/icon_${s}x${s}@2x.png`, s * 2, s * 2, img('scout-icon.svg', s * 2), true);
  }
  // Icon Composer layer: route alone, transparent, already at the system plate's scale.
  await shot('scout-layer-1024.png', 1024, 1024, img('scout-layer.svg', 1024), true);
  // Web: full-bleed tile. 180 opaque (iOS fills transparency with black); 192/512 maskable — the
  // tile fills the canvas and the route crosses the whole plate, which is what maskable expects.
  for (const s of [32, 180, 192, 512]) await shot(`favicon-${s}.png`, s, s, img('scout-favicon.svg', s));
  // Tiles for READMEs, transparent outside the rounded corners.
  for (const v of ['dark', 'light']) await shot(`scout-tile-${v}-256.png`, 256, 256, img(`scout-tile-${v}.svg`, 256), true);
  // Lockups at 2x, and the 1280×640 social preview.
  for (const v of ['dark', 'light']) {
    const svg = fs.readFileSync(path.join(here, `scout-lockup-${v}.svg`), 'utf8');
    const [, w, h] = svg.match(/viewBox="0 0 (\d+) (\d+)"/);
    await shot(`scout-lockup-${v}.png`, +w, +h, `<img src="${uri(`scout-lockup-${v}.svg`)}" width="${w}" height="${h}" style="display:block">`, true);
  }
  await shot('social-preview.png', 1280, 640, `<img src="${uri('scout-social.svg')}" width="1280" height="640" style="display:block">`);
  await browser.close();
})();
