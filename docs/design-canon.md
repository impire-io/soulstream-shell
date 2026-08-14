# The design canon (vendored 2026-08-14)

Vendored verbatim from the Soulsystem Design System project's readme
(the ecosystem's design source of truth — cassette futurism in a light
key), so builders in this repo hold the contract without leaving it.
The tokens in `internal/shellserver/assets/tokens.css` implement the
variables this document names. When this file and the Claude Design
project disagree, the project wins and this file gets re-vendored.

---

**Voice.** Plain, technical, slightly deadpan. "You" for the operator;
"the system" for the AI. Label surfaces by what they do. Sentence case
for headings and buttons; UPPERCASE only for mono label strips, badges
and tab labels. All ids, timecodes and ratios in mono. Emoji: never —
status is carried by LED pips, badges and glyphs. Never say "powered
by AI", "magic", "effortless", "revolutionary", "just", "simply".

**Palette.** A warm, light, plastic world. Seven shell tones carry
every surface; ink is warm charcoal `#1B1917`, never pure black. Two
accents at deliberately **equal weight**: amber `#E07B26` = the human
channel, teal `#1E7A72` = the machine channel — peers; neither may
outrank the other on a screen. Signal colours only as state, always
beside a glyph. Max two background tones per screen.

**The one dark surface.** `--surface-screen` (CRT glass with phosphor
and scanlines): live output, telemetry, transcript excerpts. Never
prose.

**Type.** Archivo (display bold, `-0.03em`; wordmark lowercase `wdth`
88) for anything human-readable; JetBrains Mono for data, ids, and
every label strip (11px uppercase at `0.14em`). Scale: 11 · 12 · 13 ·
15 · 17 · 20 · 25 · 32 · 42 · 56 · 76.

**Spacing.** 2 · 4 · 6 · 8 · 12 · 16 · 20 · 24 · 32 · 40 · 56 · 72 ·
96. Cards pad 24, gutters 16, control gaps 12. Rail 248px, content max
1080px, measure 66ch. Instrument-panel density: dense, never cramped.

**Backgrounds.** Flat shell colour plus one of three native textures:
`--texture-grain` (page), `--texture-rib` (header bars, card top
edges), `--texture-scanline` (CRT only). No gradients as decoration,
no photography.

**Radii.** Plastic is barely rounded: 2 / 3 / 5 / 8 / 12px. Pills are
reserved for tags and LED pips — a pill-shaped button reads as
software, not hardware.

**Cards.** 1px hairline (`#D9D0BE`) or hard ink outline; 5px radius;
`--shadow-card` = one crisp etch line + a short 6px shadow. Inset
cards use `--bevel-inset`, no drop shadow. No card floats on a blurred
cloud.

**Depth.** Crisp white top highlight, hard 2px colour shadow below,
optional short soft shadow. Inner shadow for anything a finger would
press; outer for anything that sits on top.

**Milled lettering.** Text on a coloured key is engraved
(`--engrave-light` / `--engrave-dark`). Amber keys use `--amber-600`
under light text — black-on-orange is banned.

**Hover.** Never opacity — surfaces step one shade darker, ghost
controls gain `--surface-inset`, links move amber. **Press.**
`translateY(2px)` and the shadow swaps to `--bevel-inset` — the key
travels into the housing. Nothing scales.

**Animation.** Mechanical and short: 70/120/180/280ms on
`--ease-mech`. No bounces, no springs. All durations collapse under
`prefers-reduced-motion`.

**Transparency.** Almost none — the modal scrim only. No frosted
glass, no backdrop-filter. Protection is a solid capsule, never a fade
over content.

**Borders.** Everything has an edge: hairline for quiet separation,
ink for interactive or structural, etch for the shadow line.

**Icons.** Lucide, via the vendored set, monochrome, inheriting text
colour. Transport vocabulary carries the brand where a meaning exists:
play, pause, rewind, eject, mic, radio, power. Never emoji, never
unicode glyphs as icons.

**The house readout.** The segmented VU **Meter** replaces any
conventional progress bar — level, load, capacity, parity.

**Layout.** The top transport bar is fixed with an ink bottom border;
rails and side panels sticky. Content left-aligned; centred text only
in modals and empty states.
