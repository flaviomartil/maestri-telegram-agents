# Using Maestri Relay

Relay connects one Telegram group to one Maestri installation. It creates a `Maestro` topic and `Workspace · Floor` topics for authorized workspaces. Agents on the same floor share a topic; replying to an agent's card selects the recipient by ID. Messages without a reply require exactly one active coordinator on the floor.

## Set up and connect

Requires Go 1.25, Make, Maestri with Wire enabled, and a Telegram bot that is an administrator of a supergroup with topics and permission to manage them. Install the agent presets used by your partituras on the host. A C compiler is also required for race detector tests.

```sh
make build
bin/maestri-tg init
```

Edit `~/.config/maestri-relay/config.json`; `init` creates a configuration skeleton without overwriting existing files. Set:

| Field | Value |
| --- | --- |
| `wireURL` | The HTTPS origin shown by the Wire host, reachable from the machine running Relay. |
| `wirePin` | SHA-256 of the host's SPKI public key, in hexadecimal or base64. Verify it directly in Maestri. |
| `chatID` | The negative ID of the single Telegram supergroup. |
| `operators` | Positive numeric IDs of users allowed to control agents. |
| `observers` | IDs of users allowed to use `/help` and `/status` in General. |
| `workspaces` | Allowed workspace IDs. `[]` allows none; `["*"]` allows all and permits creating new ones. |
| `stateDir` | Absolute path for state and pairing, dedicated to this group/host. |
| `catalogDir` | Absolute path for the downloaded guide catalog. |
| `pollSeconds` | Polling interval; defaults to 5 seconds. |
| `llmURL` | OpenAI-compatible base URL, such as `https://api.openai.com/v1`; the client appends `/chat/completions`. Optional when applying templates without adaptation. |
| `llmModel` | A model identifier available from the configured provider. |

Set `TELEGRAM_BOT_TOKEN` in the process environment. For AI, set `MAESTRI_LLM_KEY` as required by your provider. Keep secrets out of the JSON configuration and repository.

Enable pairing in Maestri and run:

```sh
bin/maestri-tg pair
bin/maestri-tg doctor
bin/maestri-tg catalog-sync
bin/maestri-tg run
```

`pair` asks for the six-digit code and saves the token to `stateDir/wire-token` with `0600` permissions. Alternatively, set `MAESTRI_WIRE_TOKEN` in the environment. Use an `owner` pairing for creation and control. `doctor` checks connectivity, protocol and role; each feature depends on the capabilities advertised by the host.

`run` keeps the daemon running in the terminal; use your usual process supervisor for continuous operation. Run only one Telegram update consumer per token. The local lock prevents two processes from sharing `stateDir`, but does not protect installations on different machines or in different directories. The upstream Herdr installers do not install Relay.

## Telegram commands

| Command | Behavior |
| --- | --- |
| `/status`, `/workspaces`, `/floors` | List known floors with links to their topics. |
| `/agents` | Post cards for the floor's agents; reply to the desired card. |
| `/screen` | Show the text preview available in the feed. |
| `/focus` | Reveal the node in Maestri. |
| `/stop`, `/interrupt` | Send Escape or Ctrl+C to the selected terminal. |
| `/close` | Ask for confirmation before terminating the process. |
| `/mute`, `/unmute` | Control notifications for that floor. |
| `/partituras description` | Search the entire catalog; show up to 12 results per query. |
| `/apply ID` | Apply the example on the current topic's floor. |
| `/adapt ID description` | Adapt an example with AI and create the team. |
| `/create description` | Create a team from a description. |
| `/preview description` | Return a JSON plan without creating resources. |
| `/resume ID` | Resume a creation job using previously recorded results. |
| `/bind workspace-id floor-id\|ground topic-id` | Manually recover a topic binding after an uncertain topic creation response. |

In `Maestro`, for example: “Create a Review floor in the Demo Project workspace with one coordinator and two Codex reviewers.” To create a workspace, also specify its directory explicitly and allow `"*"` in the configuration. Inside floor topics, the destination is fixed to that topic; use Maestro to request a different destination.

The planner handles questions and requests for proposals without creating resources. Operators can create resources directly from descriptions; Relay validates the plan and only permits installed presets. Prompts, descriptions, workspace names and the example being adapted are sent to the configured AI provider. Files up to 8 MiB can be sent to the selected agent, including captions.

Bot and CLI messages currently use Brazilian Portuguese. Natural-language requests can be written in English. The default ground-floor topic label is `Térreo`.

## Catalog and portals

Source: [Maestri Guide](https://github.com/arthurspk/guiadomaestri), commit `24ccb073c761864d3352552053a38240f70224a1`. `catalog-sync` downloads this pinned revision: 257 partituras and 115 references, totaling 372 entries. Future guide changes are not included until the revision is updated and validated.

All partituras in this revision are converted, preserving terminal, note and portal components, roles, positions and connections. Launch commands from the guide are replaced with installed presets. The other 115 entries are available as reference notes, including recipes, prompts, instructions and theme/workspace examples. This does not implement automatic recipe execution or import theme/workspace settings.

Terminals and notes are created through Wire endpoints. Portals use native application of an installed partitura. To prepare the entire library:

```sh
bin/maestri-tg catalog-export --out /tmp/Relay.maestripartituras
```

This command requires a paired Wire host and installed presets compatible with every agent in the catalog. Import the file through Maestri's partitura library. Then `/apply ID` applies the templates. If a variant containing portals is not installed, the bot supplies `Relay.maestripartitura` and a creation job ID: import it, then run `/resume ID`. Adapting the composition or changing a preset can produce a variant requiring another import.

The documented Wire API can list/apply partituras, but cannot import, save or edit the library, or create a portal directly. Fully automatic creation of every portal variant is therefore not yet possible through this API. No redistribution license was found for the guide; its content is downloaded locally instead of bundled with this fork. Preserve attribution and check redistribution rights before sharing generated files.

## CLI plans and recovery

All commands accept `--config PATH`. List IDs with `catalog-list --query description`. A catalog-based plan can be generated without Wire:

```sh
bin/maestri-tg plan --template ID --workspace WORKSPACE_ID --floor FLOOR_ID --out /tmp/team.json
bin/maestri-tg apply --plan /tmp/team.json --job review-001
```

Omit `--floor` for the ground floor. For AI planning, use `plan --request "description"`; this reads workspaces, floors and presets from Wire without creating resources. Review the JSON. A plan with `action: "preview"` must explicitly be changed to `action: "create"` before `apply`; answers and other actions are rejected.

Keep the same `--job` and plan when resuming a creation. The executor records each step before sending its mutation and each received result before continuing. A timeout after sending leaves the step uncertain and blocks automatic retries. `mutationId` identifies the operation; it is not treated as an idempotency guarantee. In this situation, inspect the canvas and `stateDir/relay.json` before manual recovery. Do not switch job IDs to bypass uncertainty, as this may duplicate resources.

New floors are created without Git isolation; this version has no branch creation option. Each provisioning execution has a five-minute deadline. Verification confirms the IDs of directly created nodes; native application checks new nodes by type against the previous canvas. It does not prove visual fidelity or resource identity during concurrent host edits.

## Validation and remaining work

```sh
make test
make build
make lint
```

`make lint` requires Staticcheck compatible with Go 1.25 and checks five build targets. To validate the complete downloaded corpus:

```sh
MAESTRI_GUIDE_TEST_DIR="$HOME/.cache/maestri-relay/guide/24ccb073c761864d3352552053a38240f70224a1" go test -v ./internal/adapters/maestri -run TestFullGuideCatalog
```

Tests cover TLS/SPKI, permissions, ID-based routing, stale replies, forged callbacks, persistence failures, resuming without replay, and catalog conversion. No pairing or messages to a real Telegram/Maestri installation were performed for this implementation. Two workspaces with multiple floors, agents sharing names, focus changes and host restarts still need live validation.

Relay does not yet reproduce the full Herdr feature set: complete terminal history/emulation, Git commands, album aggregation, a pinned dashboard, presence and all upstream preferences remain pending. `/screen` shows a preview. The API provides `epoch`, but no documented per-process generation precondition for restarts that reuse the same ID. Telegram updates preserve the remote queue on startup; the receive buffer remains in memory without a durable inbox. Recorded messages are deduplicated for up to 48 hours; a failure after recording but before delivery may require an explicit resend.
