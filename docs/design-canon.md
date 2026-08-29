# The design canon (vendored 2026-08-29 — the second canon)

Vendored from the New Impire Design System project (the ecosystem's
design source of truth — paper and ink, one accent; adopted
visual-for-visual by design soul-hq/02-DESIGN/soulstream-shell/0011),
so builders in this repo hold the contract without leaving it. The
tokens in `shell/assets/tokens.css` implement the variables this
document names. When this file and the Claude Design project disagree,
the project wins and this file gets re-vendored. "Done when there is
nothing more to take away."

---

**Voice.** Plain, technical, slightly deadpan. "You" for the operator;
"the system" for the AI. Label surfaces by what they do. Sentence case
for headings and buttons; UPPERCASE only for mono label strips, badges
and column heads. All ids, timecodes and ratios in mono. Emoji: never —
status is carried by dots, chips and glyphs. Never say "powered by
AI", "magic", "effortless", "revolutionary", "just", "simply".

**Palette.** Paper and ink. Surfaces are warm-cool neutrals — the page
`#FAFAF7`, cards white, sunken `#F2F2EE` — never more than two
background tones per screen. Ink is a near-black with a hint of
blue-graphite (`#161B22` for type and edges, `#0B0E13` at the very
bottom), never pure black. **One accent**: teal `#1FA88E` — the agent
inside the brackets — for primary actions, focus rings, active
markers, live dots; used sparingly, one accent per view. Red
`#B23A3A`, green `#2F7A4F`, blue `#2F5C9E` and the warm warn tones are
system signals only, never decoration, always beside a word or glyph.

**The one dark surface.** `--surface-screen` (brand ink with
teal-tinted text): raw output, payloads byte for byte, live tails.
Never prose. No scanlines, no glow — the glass is just dark.

**Type.** Geist — a single family, weight range 100–900; thickness is
the play, never style. Headings are medium (500), tracked tight
(display `-0.02em`); body is regular at 15/1.5. Geist Mono for data,
ids, and every label strip (11px uppercase at `0.14em`). Scale: 11 ·
12 · 13 · 15 · 16 · 18 · 22 · 28 · 36 · 48 · 64.

**Spacing.** A 4px base scale: 2 · 4 · 6 · 8 · 12 · 16 · 20 · 24 · 32
· 40 · 56 · 72 · 96. Cards pad 24, gutters 16, control gaps 12.
Sidebar 232px, content max 1180px, measure 66ch. Calm density: airy,
never cramped.

**Backgrounds.** Flat colour. No textures, no gradients as decoration,
no photography.

**Radii.** Small — candor, not pillow: 2 / 4 / 6 / 10 / 12px. Pills
are reserved for tags, counts and dots — a pill-shaped button reads as
somebody else's product.

**Cards.** White on paper; 1px hairline (`--ink-200`); 10px radius;
`--shadow-card` is one crisp line and a 2px soft edge. No card floats
on a blurred cloud.

**Depth.** A border first. Two small shadows exist for what genuinely
sits on top (panels, drawers, the modal); nothing is bevelled, nothing
is engraved, nothing travels when pressed.

**Hover.** Never opacity — surfaces step one shade darker (`--surface-
inset`), quiet keys gain the sunken tone, links harden their
underline. **Press.** The key darkens one more step (`--accent-press`).
Nothing moves, nothing scales.

**Animation.** Fast, ease-out, no bounce: 80/140/220/360ms on
`--ease` (`cubic-bezier(.4,0,.2,1)`). All durations collapse under
`prefers-reduced-motion`.

**Transparency.** Almost none — the modal scrim only. No frosted
glass, no backdrop-filter. Protection is a solid panel, never a fade
over content.

**Borders.** Everything separable has an edge: hairline (`--ink-200`)
for quiet separation, the firmer `--ink-300` for controls and selected
things, ink itself (`--ink-900`) only where a thing must be findable —
a mention, a quiet key's outline.

**Icons.** Lucide, via the vendored set, monochrome, inheriting text
colour — 24×24 line convention. Never emoji, never unicode glyphs as
icons.

**The accent's meaning.** Teal is the agent's: live dots, operated
voices, the one filled key, the active place in the sidebar. Whose a
message is rides alignment; who answers for a voice rides words and
its dot — colour never carries identity beyond that one lamp.

**Layout.** A labeled sidebar on paper at the far left — wordmark at
the top, the signed-in name at the foot; no top bar. Content
left-aligned; centred text only in empty states and the sign-in card.
