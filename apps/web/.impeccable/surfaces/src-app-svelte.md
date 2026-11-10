---
version: 1
slug: "src-app-svelte"
primary_target: "src/App.svelte"
related_targets: []
---

# Kantong app shell (all routes)

Scope: the whole PWA (Beranda, Transaksi, Kalender, Budget, Analitik, Aset, Review, Aturan, sheets). Mode: Operate. Redesign of layout and look; every feature stays, placement may move.

Audience and job: the owner, on phone and desktop equally; check what's left this month, review auto-imported transactions, log one by hand, keep budget and assets current.

Constraints: no new dependencies; system font stack; light and dark follow the system; hide-amounts toggle covers every figure; Indonesian copy unchanged except new navigation labels.

## Direction contract

THESIS: A familiar personal-finance app (Money Lover, Monarch/Copilot, Jago/BCA) built cleanly: icon tab bar with a center add button, a colored balance card on top, category icons on every row. Refuses the current spreadsheet look: grey cards, text-only rows, scattered navigation.

OWN-WORLD: Deep green brand (#1f6f5c light / #4fb89a dark) owns the balance card and the add button; neutral cool-grey ground with white cards (16px radius, soft shadow in light, elevated surface in dark). Each category has a hue: a tinted circle with a drawn line icon. Green for income, red for over-budget, amber for review. System sans, tabular figures, bold amounts.

STORY: Opening Beranda the owner sees what's left to spend and net worth at once, notices anything to review, scans recent transactions by icon, and taps + to add.

FIRST VIEWPORT: Mobile: month switcher row; green card with "Sisa bisa dibelanjakan" big, budget track, Masuk/Keluar/Selisih strip; review alert row; net-worth and analytics shortcut cards; recent transactions with icons. Bottom tab bar: Beranda, Transaksi, center +, Budget, Aset. Desktop: icon sidebar (all routes grouped, review count), same cards in two columns.

FORM: The category standard, played straight (user-pinned canon, references Money Lover, Monarch/Copilot, bank apps). seed: none (user-pinned canon, no roll).

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
