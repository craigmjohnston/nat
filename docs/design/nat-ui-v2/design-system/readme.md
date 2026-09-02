# macOS Native Design System

A design system for building modern macOS applications that feel indistinguishable from Apple's own (Notes, Mail, Finder, Freeform). It encodes the platform's semantic styles — colors, type ramp, materials, control metrics — as web tokens and React recreations, so prototypes and mocks read as genuinely native.

**Source:** `macos design/DESIGN.md` (attached local folder, read-only) — the sole input. No Figma, no app codebase, no brand assets were provided.

This is a *platform* system, not a brand: there is no logo, no brand color, no custom typeface. The "brand" is deference — quiet translucent chrome, opaque legible content, the user's own accent color driving all tint.

## Principles (from DESIGN.md)
- Native-first: use the system component; never rebuild chrome.
- Deference to content: chrome is quiet and translucent, content opaque.
- Semantic, not literal: every color/font/material referenced by role, never hex. (The token files here hold the web approximations of those roles — consumers still reference only the tokens.)
- Mac density: 13px body, 22px controls, compact pointer-driven layouts. Never iOS metrics.
- Keyboard-equal: every action reachable by keyboard; focus rings never suppressed.

## Content fundamentals
- **Voice:** quiet, factual, instructional. The interface never talks about itself; text tells the user what things are and what actions do.
- **Casing:** Title Case for commands — menu items, buttons, toolbar labels ("Show Sidebar", "Delete", "New Note"). Sentence case for descriptive text — form footers, alert messages, empty-state descriptions ("New mail will appear here.").
- **Buttons and menu items are verb phrases.** Alerts use a clear verb-labeled action ("Delete", "Don't Save") — never "OK" for a destructive choice. Ellipsis (…) on commands that open a further dialog ("Rename…").
- **Person:** address the user as "you" sparingly, in descriptions only; never "I". No exclamation marks, no marketing tone, no humor in chrome.
- **Emoji:** never in interface chrome. Emoji appear only inside user content.
- **Numbers:** plain numerals with tabular figures in lists and tables; relative dates where the platform does ("Yesterday", "Tuesday", "Aug 21").
- **Keyboard shortcuts** are shown with symbols (⌘⇧R, ⌘⌫) next to their menu items.

## Visual foundations
- **Color:** zero literal hex in interfaces — semantic tokens only. Text is 4 alpha-based label levels; surfaces are window (#ECECEC/#282828-ish), opaque white/near-black content, gray under-page. One accent drives all selection/tint and belongs to the user (blue by default, never assume, never brand). Status uses the 11 system tints, always paired with a glyph or label. Never pure #000/#FFF.
- **Type:** system font only (`-apple-system`), 13px body, ~10–26px ramp, hierarchy by label color and weight before size (≤3 sizes per view), system line-heights, no custom tracking. Monospaced digits in tables/timers; SF Mono stack for code.
- **Backgrounds:** no imagery, no illustration, no patterns, no decorative gradients. Chrome = translucent materials (blur + saturate) over whatever is behind; content = opaque `--control-bg`. Every material has an opaque fallback for Reduce Transparency.
- **Spacing:** 4/8/12/16/20 grid; 20px content margins; 8px between related controls. Dense pointer-driven metrics: 22px controls, 24–28px rows (44px only for rich two-line rows).
- **Borders & shadows:** 0.5–1px hairline separators (`--separator`); panels get hairline stroke + soft drop (`--shadow-menu/popover/window`); controls get a subtle 0.5px stroke-and-drop (`--shadow-control`). No heavy outlines.
- **Corners:** 6px controls and fields, 5px selection/menu highlights, 10px panels/windows/group boxes.
- **Cards:** rare; the equivalent is the grouped form box — 10px radius, opaque `--control-bg`, hairline inset stroke, hairline row separators. No floating drop-shadow cards in content.
- **Hover:** quiet — quaternary-label wash on borderless/toolbar/sidebar items; native controls need no custom hover. **Press:** slight darken (brightness ~0.94 or a pressed face token); no shrink/scale.
- **Selection:** accent-filled rounded highlight (white text) in key windows; gray `--selected-content-bg-unemphasized` when the window isn't key.
- **Motion:** 0.15–0.35s, default/ease-out curves, springs only for interactive transitions. Animate state changes (collapse, selection, reorder); no entrance animations, no parallax, no bouncing chrome. Honor Reduce Motion with cross-fades.
- **Transparency & blur:** the signature of chrome — sidebar, header/toolbar, menus, popovers, HUD. Never under content, never relied on for legibility.
- **Focus:** system-style 3.5px accent-tinted ring (`--focus-ring`), never suppressed.

## Iconography
- The mandate is **SF Symbols for everything** — monochrome in chrome (secondary label at rest, accent when selected), hierarchical for large empty-state glyphs, filled variants in sidebars, outlined in toolbars, scale/weight matched to adjacent text.
- **Web substitution (flagged):** SF Symbols are not redistributable, and no icon assets were provided, so the `Icon` component loads **Framework7 Icons** from CDN (`cdn.jsdelivr.net/npm/framework7-icons@5.0.5`) — a ligature icon font drawn to Apple's style with SF-like names (`gear`, `square_arrow_up`, `chevron_down`). In real SwiftUI/AppKit work, use actual SF Symbols; the names mostly transfer.
- No icon font of the brand's own, no bundled SVGs, no emoji-as-icons, no unicode-glyph icons (except ⌘⇧⌫ etc. in shortcut labels, which are text).
- No logo exists in the sources; render app/product names in plain system type wherever a mark would go.

## Intentional additions
- `Icon` — wrapper for the glyph set (needed because SF Symbols can't ship on the web).
- `TrafficLights` (in Toolbar) — window controls, needed to draw believable window chrome.

## Components
Namespace: `window.MacOSNativeDesignSystem_153403` (see each `*.prompt.md` for usage).
- **buttons/** Button
- **controls/** Checkbox, RadioGroup, Switch, PopUpButton, TextField, SearchField, Stepper
- **navigation/** Toolbar, ToolbarButton, TrafficLights, Sidebar, SidebarSection, SidebarItem
- **data/** List, ListRow, Table
- **forms/** Form, FormSection, FormRow
- **presentation/** Sheet, Popover, Alert, Menu, MenuItem, MenuSeparator
- **feedback/** EmptyState, Badge
- **icons/** Icon

## Index
- `styles.css` → `tokens/` — colors, typography, spacing, materials, shadows, motion, interaction. All tokens are `light-dark()` pairs; set `color-scheme: dark` on any subtree to flip it.
- `guidelines/` — foundation specimen cards (type ramp, labels, surfaces, tints, materials, shadows, spacing, window anatomy, motion).
- `components/` — the native inventory as React primitives (list above), one `@dsCard` per directory.
- `ui_kits/mac-app/` — interactive three-column Notes-style app composing every family; also a starting point.
- `SKILL.md` — agent skill entry point.
- Source brief: `macos design/DESIGN.md` (attached read-only folder).

## Caveats
- No font binaries ship — the platform font is mandated and SF Pro may not be bundled; the `-apple-system` stack renders SF Pro on Macs and the closest system face elsewhere. Intentional, not missing.
- Color values are careful approximations of AppKit dynamic colors (no published hex).
- Icons are a flagged CDN substitution (see Iconography).
