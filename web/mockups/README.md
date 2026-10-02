# UI mockups

Static HTML for the redesign. They are **not** served by Beacon and are **not** embedded in the Go binary.

## Run locally

From the repository root:

```sh
make mockups
```

Then open [http://127.0.0.1:8765/web/mockups/](http://127.0.0.1:8765/web/mockups/).

Or without Make:

```sh
python3 -m http.server 8765
```

same URL.

You can also open `web/mockups/index.html` as a file, but some browsers restrict SVG from `file://`. The local server is the reliable path.

Fira Sans and Lekton load from Google Fonts. Without a network they fall back to system UI / monospace.

## What to check

- Light / Dark toggle (persists in this browser)
- Desktop ≥ 1024px: sidebar + table; collapse the sidebar to icons
- Mobile: bottom dock + occupant cards
- Login: large Rainbow Haven lockup
- Print on the report page (hide chrome)

These screens follow [docs/ui.md](../../docs/ui.md). Production templates still use the old CSS until a later change.
