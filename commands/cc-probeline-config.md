---
description: Configure cc-probeline (table rows, display mode, background refresh, probes) through one interactive widget
---

# Configure cc-probeline: /cc-probeline-config

Interactive configuration for **cc-probeline**, shown as **one** native question widget with **four** questions (arrow-navigable). The user checks **only what they want to change** — anything left untouched stays as it is — and all changes are applied in a single batch at the end via the `cc-probeline` CLI setters. Never edits the TOML config directly.

## Usage

```
/cc-probeline-config
```

No arguments.

---

## Step 0 — Preconditions + read current state

1. If `cc-probeline` is not found in PATH, tell the user the binary is not installed
   yet and suggest running `/cc-probeline-install` to install it, then stop:

   ```
   cc-probeline binary not found in PATH.
   Run /cc-probeline-install to install it, then run /cc-probeline-config again.
   ```

2. Read the current effective config (needed to show 🟢/🔴 and the current table size):

   ```bash
   cc-probeline config show
   ```

   This prints JSON. Extract these values:
   - `general.table_rows` (int)
   - `general.mode` (`"standard"` | `"super-compact"`)
   - `general.tutorial_hints` (bool)
   - `general.price_check` (bool) — once-per-day network price/version check (opt-out)
   - `general.usage_refresh` (bool) — background refresh of Claude Code's usage cache (model-scoped weekly limit + extra-usage figures)
   - `widgets.model`, `widgets.cost`, `widgets.project`, `widgets.email`, `widgets.time`, `widgets.ctx`, `widgets.quota`, `widgets.git` (bool each)

---

## Step 1 — Ask all four questions in ONE AskUserQuestion call

Call the **AskUserQuestion** tool **once** with the four questions below (do NOT make four separate calls, and never print a plain-text menu).

Fill every "currently …" description and 🟢/🔴 marker from the values read in Step 0 (🟢 = currently on/shown, 🔴 = currently off/hidden). For toggles, the wording must state the flip direction (e.g. "Currently on. Check to hide.").

**Option spacing & indent (required for readability — applies to ALL four questions):** every option's `description` MUST begin with a literal tab (`\t`) and end with a literal newline (`\n`):

- The **leading `\t`** indents the description text (and its 🟢/🔴 marker) from the line start.
- The **trailing `\n`** makes the native widget render a blank line *between* options.
- Keep each description to a **single line** — never add a second description line. The blank line must fall *between* options, not inside one (a `\n` in the middle would split one option's text instead).

So each description has the shape `"\t<🟢/🔴 marker + wording>\n"`. These are verified levers for the `AskUserQuestion` widget.

### Question 1 — `Table rows` (single-select)

- **header:** `Table rows`
- **question:** `Rows in the per-turn table? Current value is first — pick it to keep. (Other = custom, 1–40)`
- **options:** list the **current value first**, labelled `"<N> (current)"`, then three other presets chosen from {5, 10, 15, 20} that are not the current value. The widget auto-offers an **"Other"** free-text choice — rely on it for a custom number (do not add a separate prompt). The binary clamps any value to **1–40**.

Example when current = 10:
- `10 (current)` — keep the current value, nothing changes.
- `5` — minimal table.
- `15` — taller table.
- `20` — max useful height.

### Question 2 — `General` (multi-select, flip)

- **header:** `General`
- **question:** `General settings. Check ONLY what you want to change — anything unchecked stays as it is. (🟢 = currently on, 🔴 = currently off)`
- **multiSelect:** true
- **options** (4):
  - **Display mode** — description reflects current: `🟢 Now: standard. Check to switch to super-compact.` (or the reverse when current is super-compact).
  - **Limit refresh (background)** — `🟢 Currently on. Check to turn off (no background process; model limit and extra usage go dark).` / `🔴 Currently off. Check to turn on (refreshes limits every 5 min).` per `usage_refresh`.
  - **Tutorial hints** — `🟢 Currently on. Check to turn off.` / `🔴 Currently off. Check to turn on.`
  - **Price check (network)** — `🟢 Currently on. Check to turn off (stay fully offline; baked prices).` / `🔴 Currently off. Check to turn on (daily price/version check).` per `price_check`.

### Question 3 — `Probes 1` (multi-select, flip)

- **header:** `Probes 1`
- **question:** `Probes (1 of 2). Check ONLY the probes you want to flip — unchecked = leave as-is. (🟢 shown / 🔴 hidden)`
- **multiSelect:** true
- **options** (4): `email`, `project`, `quota`, `git` — each description: `🟢 Currently shown. Check to hide.` or `🔴 Currently hidden. Check to show.` per its current value.

### Question 4 — `Probes 2` (multi-select, flip)

- **header:** `Probes 2`
- **question:** `Probes (2 of 2). Check ONLY the probes you want to flip — unchecked = leave as-is. (🟢 shown / 🔴 hidden)`
- **multiSelect:** true
- **options** (4):
  - **model** — `🟢 Currently shown. Check to hide.` / `🔴 …show.` (this toggle controls **both** the model name and the effort indicator).
  - `ctx`, `cost`, `time` — each per its current value.

---

## Step 2 — Apply the choices (single batched run)

Translate the answers into CLI setters and run them in **one** chained bash command (`&&`). Skip anything the user did not change.

- **Table rows:** if the chosen value differs from the current one, add `cc-probeline table-rows <N>`.
- **Display mode** checked → `cc-probeline mode <the other value>` (standard ↔ super-compact).
- **Limit refresh (background)** checked → `cc-probeline usage-refresh off` if currently on (`usage_refresh` true), else `cc-probeline usage-refresh on`.
- **Tutorial hints** checked → `cc-probeline hints off` if currently on, else `cc-probeline hints on`.
- **Price check (network)** checked → `cc-probeline price-check off` if currently on (`price_check` true), else `cc-probeline price-check on`.
- **Each probe** checked → flip it: `cc-probeline widgets <name> off` if currently shown, else `cc-probeline widgets <name> on`.
  - **`model`** is special: emit **two** commands with the same new state — `cc-probeline widgets model <state>` **and** `cc-probeline widgets effort <state>`.

If nothing was changed, do not run any setter — just print the confirmation.

After the setters run, output exactly one line:

```
Settings saved. Changes take effect on the next status-line refresh.
```

---

## Rules

- **One AskUserQuestion call, four questions.** Never split into multiple calls or print plain-text menus.
- **Spacing & indent:** prefix every option `description` with `\t` and suffix it with `\n` (leading tab = indent, trailing newline = blank line between options). Single-line descriptions only — no second line.
- **Flip semantics:** an unchecked toggle means "leave unchanged". Only checked toggles are flipped. The default (nothing checked) changes nothing.
- **Never hand-edit TOML.** All writes go through the CLI setters.
- **Fill current state from `cc-probeline config show`** — the 🟢/🔴 markers and the "(current)" table-rows option must reflect real values, otherwise the flip is unsafe.
- User-facing wording calls them **probes** (the CLI setter is historically `widgets`).
