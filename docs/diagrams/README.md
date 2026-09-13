# Usage diagram

[README image](maestri-relay.png) · [Interactive HTML](maestri-relay.html) · [JSON source](maestri-relay.architecture.json)

The main path shows commands traveling from Telegram to Maestri. Responses return through the same Relay. The branches show the catalog, optional AI provider and local state. The diagram reflects `internal/app/relay*.go`, `internal/compose/relay.go` and `internal/adapters/maestri/`; it does not represent a real installation or contain user IDs or credentials.

Download the HTML and open it directly in a browser. Use chapters to highlight conversations or team creation, the theme button to switch between light and dark, and Export to generate PNG or SVG. The README image was produced by this exporter. The optional font comes from Google Fonts; offline, the browser uses a local fallback. Diagram text, viewer controls and `html lang` are English.

## Update with Archify

Install the [Archify](https://github.com/tt-a1i/archify) skill. Set `ARCHIFY_DIR` to the installed package directory. After editing the JSON, run these commands from the repository root:

```sh
node "$ARCHIFY_DIR/bin/archify.mjs" validate architecture docs/diagrams/maestri-relay.architecture.json --quality showcase --json
node "$ARCHIFY_DIR/bin/archify.mjs" deliver architecture docs/diagrams/maestri-relay.architecture.json docs/diagrams/maestri-relay.html --quality showcase --json
node "$ARCHIFY_DIR/bin/archify.mjs" visual-check docs/diagrams/maestri-relay.html --json
```

After validation, open the HTML, inspect both themes and export the PNG again. GitHub displays the image in the README; the HTML must be downloaded or hosted separately to run its controls.

## Evidence for this version

Archify 2.16, diagram type `architecture`: validation and delivery passed **9/9 showcase checks, with zero errors and zero warnings**. The English translation retained the existing layout and required no additional geometry corrections.

| Artifact | SHA-256 |
| --- | --- |
| JSON | `a88bb178db15240c7f9dec7ec6231906df41ab5748c64ea4382a8aa1f999b82b` |
| HTML | `62bd732a3d5bcece092da9137cf8d29baa1253d747d8b25f13f68993d9d00304` |

Visual review passed in Chromium through `agent-browser`, with light/dark captures inspected at 1440×900 and 2048×1320. The settled page also remained within the viewport at 1600×1000 and 1920×1080. The exported PNG was opened and inspected.

The package's automated `visual-check` command did not complete on this machine because `Page.loadEventFired` timed out; its result is not reported as passing. The measurements and captures above used a separate browser session on the same HTML, without modifying the artifact.
