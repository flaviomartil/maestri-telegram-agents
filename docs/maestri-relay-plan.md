# Maestri Relay: fork naming and implementation plan

Implementation status as of September 13, 2026: Wire adapter, floor topics, fully indexed catalog, planner and executor implemented locally. This document preserves the approved scope, including outstanding criteria. See [setup and current limitations](maestri-relay-setup.md), especially portal imports, non-executable references and the live host validation still required.

The proposed name is **Maestri Relay**, with repository `maestri-telegram-agents` and binary `maestri-tg`. The name describes the bridge between Telegram and Maestri, while the repository name retains a clear connection to the original project.

Other options:

| Name | When to choose it |
| --- | --- |
| Maestri Remote | A direct name for controlling agents from a phone. |
| Maestri Telegraph | A distinct identity centered on messages and notifications. |
| Maestri Conductor | Emphasizes coordinating agents across projects. |
| Maestri Telegram Agents | Maximum clarity, close to the original name. |

Name and trademark availability have not been checked.

## What the fork should do

One Telegram group and one bot bring together the authorized workspaces in a Maestri installation. Each workspace/floor pair has a `Workspace · Floor` topic shared by that floor's agents. There are no subtopics or separate groups per workspace.

Example topics: `Maestro`, `Demo Project · Ground Floor`, `Demo Project · Redesign` and `Demo Store · Review`. The global `Maestro` topic creates and organizes workspaces, floors and teams from descriptions or examples. General holds an index linking to the topics. These are illustrative labels; the current runtime uses `Térreo` for the ground floor.

Inside a floor topic, a message without a recipient goes to that floor's designated coordinator. Replying to an agent's message routes the response to that agent; a selector lets users address a new message to another agent. All output identifies its author. Never broadcast to all agents by default or keep a mutable global recipient shared between users.

The catalog includes **all partituras and examples from the Maestri Guide**, with description search, automatic application and AI adaptation. Complete coverage is a delivery requirement; starting with one example in a technical proof does not reduce the final scope.

“Any workspace” means that any workspace can be enabled while preserving Maestri permissions. It does not mean automatically exposing every project. Controlling other machines is outside the first version's scope.

## Existing functionality worth reusing

[herdr-telegram-agents](https://github.com/permgps/herdr-telegram-agents) is MIT licensed and uses Go 1.25. It already offers per-agent topics, status, bidirectional messaging, question buttons, terminal reading, attachments, Git commands, agent creation, operators/observers and persistent agent/topic bindings.

The code separates domain, use cases and adapters. Herdr integration is concentrated in `internal/adapters/herdr/`, with composition in `internal/compose/`, although Herdr references also appear in contracts, the CLI, environment and installer. Preserve the Telegram core and adapt the integration instead of rewriting the bot.

**The central integration uses official Maestri Wire.** Its documentation confirms workspace listing and creation, per-floor feeds, terminal interaction, floor creation, roles, preset queries and application of existing partituras. The protocol uses HTTPS/WSS, pairing and capabilities advertised by the installation.

Wire is beta: verify `protocolVersion` and capabilities on the actual host. During the initial investigation, the CLI returned `maestri: only available inside Maestri terminals (MAESTRI_SOCKET not set).`; this CLI limitation does not establish that Wire is unavailable. Pairing and live integration testing have not yet been performed.

The Maestri Guide provides the catalog and examples, not a replacement for the official contract. Some guide formats have fidelity caveats; validate against the actual host version before claiming compatibility. Check the licenses of the guide and incorporated resources before redistribution, preserving attribution.

## Recommended architecture

`One Telegram group ↔ one maestri-tg daemon ↔ Maestri Wire ↔ enabled workspaces, floors and agents`

Keep one Telegram update consumer per token. The catalog and Maestro planner belong in the same application; no separate service is needed. The model interprets requests and produces a structured definition; the executor validates that definition and uses supported Wire operations.

There are two ways to materialize a partitura:

| Option | Benefit | Limitation |
| --- | --- | --- |
| Apply a partitura already installed on the host through Wire | Preserves native application behavior. | Wire documents listing/applying partituras, but library creation/editing remains on the host. |
| Read an example or generate its adaptation, then create its components through Wire | Supports creation from descriptions without importing every request manually. | Requires support for the example's components and fidelity validation; components without an API need additional integration. |

Use the first option when the exact template is installed and the second for external examples and adaptations. Creating an arrangement on the canvas and saving a new partitura in the native library are different operations; automating the latter requires an additional supported path.

Pair the daemon with the host and pin its security key according to the official contract: SHA-256 of the certificate's public key. Do not blindly copy the guide's sample client or disable TLS verification. The executor preserves group, operator and workspace restrictions even when its Wire token has the owner role.

## Implementation plan

### 1. Validate Wire and catalog formats

Query `/api/info` and the paired installation's capabilities, then build a capability matrix: IDs, per-floor feeds, terminal reading, text/keys, status, questions, creation, focus and termination. Inventory the entire guide catalog at an identified commit and classify the components needed to reproduce each example.

Validate two workspaces, each with at least two floors, including agents sharing names. Also check behavior when another workspace is open, a floor is inactive and the application restarts. Tests that send text must use test terminals.

Deliver a compatibility matrix for Wire and all examples, with explicit gaps. Status, questions and floor isolation depend on actual host capabilities and responses, not assumptions about terminal text or the platform.

### 2. Create the fork and Maestri layer

Create the fork under the chosen name, preserve MIT licensing and attribution, and configure upstream. Keep Go and the existing dependencies.

Add `internal/adapters/maestri/`, reusing existing contracts where suitable. Adjust contracts and composition only where Maestri semantics require it. Adapt configuration, diagnostics, startup and installation so they do not depend on the Herdr environment or marketplace. Avoid renaming the entire codebase in the first delivery, making upstream fixes easier to incorporate.

Implement the adapter against the official Wire contract, checking capabilities before using optional features. The registered CLI remains available for verified local operations without a Wire equivalent. Do not assume the Herdr plugin manifest is compatible with Maestri.

### 3. Make routing independent of names and the active workspace

Persist the topic's installation, workspace and floor binding. Within it, bind messages and buttons to terminal and session IDs. Message keys include `chat_id` and `message_id`; topic keys include `chat_id` and `message_thread_id`. Separate terminal identity from session/process generation so stale buttons cannot act on a replacement session.

Names are for display; IDs resolve the destination. Renaming a workspace or floor updates the topic without losing its binding. When a terminal moves floors, reconcile its metadata and invalidate old controls before accepting commands. Every operation uses an explicit destination, never the focused workspace. If no coordinator is available or the destination is ambiguous, show a selector without sending the message.

Version state and back it up before migrations. Use a dedicated state directory to avoid mixing the fork with an existing Herdr bot.

### 4. Deliver the core Telegram flow

First delivery: connect a group, discover authorized workspaces/floors, create one topic per pair, and talk to agents using the explicit routing above. Reuse the upstream dashboard, queue and rate control; adapt persistence and reconciliation as the relationship changes from one topic per agent to several agents per topic.

The proposed commands are `/workspaces`, `/floors`, `/agents` and `/partituras`; these are now implemented. Prefer button selectors over typing ambiguous names. `/screen` and interruption commands resolve a specific agent; `/status` summarizes the floor with links to agents. Aggregate repetitive progress and prioritize questions and completions so topics remain readable.

Keep agent communication concurrent: a long task in one topic must not block others. Serialize sends to the same terminal. Deduplicate Telegram updates and callbacks; if a connection drops after a send with an uncertain result, report the uncertainty without automatically resending the task.

### 5. Complete functional parity

Add question/key buttons, interruption, attachments and media, Git operations in the agent's actual directory, agent creation on the selected floor, focus and closing, according to the capability matrix.

Preserve notification controls, presence, quiet mode and operators/observers. Agent-specific commands such as `/compact` or `/model` should only be forwarded when appropriate for that agent type. An unavailable feature must be shown as unavailable; never simulate success.

Attachments require paths accessible to the destination process. On Windows/WSL, validate path translation. On isolated floors, Git operations use that floor's checkout, never a global default directory.

### 6. Implement Maestro and the complete guide catalog

Import the full inventory of partituras and guide examples, including recipes that are not partitura files. Record source path, commit, category, description, components, parameters and dependencies. Start with text search by name/description; AI interprets intent and adapts the selected example. A vector database is unnecessary for the first version.

Offer three paths in the same Maestro topic:

- **Apply an example:** “Set up the review partitura in Demo Project, on the Review floor.”
- **Adapt an example:** “Use this partitura, but with two Codex agents, instructions in English and a note containing acceptance criteria.”
- **Create from a description:** “Build a team to investigate a payment error, with an implementer, a reviewer and an application portal.”

Resolve actual workspaces, floors, directories and presets before execution. Clear creation requests authorize the described additive operations; do not require a routine second approval. If the user asks only for a preview, generate a proposal without materializing it. Do not start agent work when the request is only to assemble a team.

Convert each request into a validated definition of nodes, roles, connections, notes, portals and environment parameters. Reuse compatible existing resources. Use execution identifiers and `mutationId` on routes that support it; reconcile results before repeating operations elsewhere. Create dependencies first, wait for pending floors, then materialize components and verify the result.

Preserve the original example and save adaptations as identified variants with provenance and a description of changes. Guide updates must not overwrite user variants. Models, presets and credentials come from the installation; examples do not authorize creating credentials or enabling nonexistent providers.

Hook, routine and environment recipes need their own contracts and verified operations, not automatic execution of guide text. Treat catalog content as data; its instructions cannot change permissions or destinations. Changes to roles already in use require evaluating terminal restarts; prefer a new role for a local adaptation.

For components without a supported API, implement and validate the necessary integration or mark the example as blocked with an exact reason. Do not omit components or claim complete coverage while examples remain pending. Applying a curated selection may be an intermediate milestone, but does not satisfy the requirement to cover every partitura and example.

After creation, Maestro sends the `Workspace · Floor` topic link, the created agents and any outstanding steps. A partial failure preserves the record of created resources for resuming without duplication; cleanup must never remove resources that predate the request.

### 7. Validate, package and document

Reuse existing tests and CI; add a simulated Maestri adapter, routing tests for agents sharing a topic, and creation/adaptation cases. Run live integration tests with two workspaces and two floors before the first release. Validate the entire catalog against the supported schema and verify different component types and flows on the host, maintaining a per-example coverage report.

Package first for Windows/WSL, documenting which side runs the daemon and Maestri. Include guided configuration, diagnostics, automatic startup through a supported mechanism, and restarting with state preserved.

Rollback: stop the fork's daemon and restore its previous binary/configuration/state. Keep upstream as a reference for Telegram updates and fixes.

## Acceptance criteria

- One group exists with one topic per workspace/floor, without individual agent topics.
- Agents sharing names across scopes receive only messages explicitly resolved to their IDs.
- Replies and buttons reach the correct originating agent, even when several agents post simultaneously in the same topic.
- Messages without recipients go to the floor coordinator; absence or ambiguity never sends them to an arbitrary agent.
- Switching the active desktop workspace does not change message destinations.
- Unauthorized users and observers cannot send commands, including through old buttons.
- Restarting the daemon preserves bindings without duplicating topics or resending processed prompts.
- Losing connectivity marks the scope unavailable without treating all agents as terminated.
- Isolated floors preserve the correct directory for attachments and Git.
- A button from a terminated session cannot control a new session in the same terminal.
- The dashboard shows which workspaces/floors are enabled, connected or suspended.
- Tokens stay out of logs; upstream secret redaction is preserved without assuming it recognizes every kind of sensitive content.
- Closing a terminal requires confirmation bound to the correct target and session.
- Every guide example at the chosen commit appears in the catalog with provenance, a description and a compatibility result; full coverage requires validated materialization without silently discarding components.
- Users can apply an example, adapt an example and create a team from a description in Maestro.
- Repeating a Telegram update or resuming interrupted creation does not duplicate workspaces, floors, teams or topics.
- Adaptations preserve originals and use available presets without modifying teams outside the request.
- Results distinguish arrangements created on the canvas from partituras saved in the native library.

## Order and effort

Validate Wire and inventory the entire guide first, followed by the group with floor topics, the Maestro creator and full catalog coverage. Complete original features alongside these where technically independent. Estimate delivery after identifying guide components that require integrations beyond Wire.

The first demonstrable milestone is: **in one Telegram group, talk to agents from two workspaces through their floor topics and create a team from a description**. This milestone validates the concept; completing the fork requires functional parity, the full catalog and the criteria above.

## Sources

- [Upstream README and features](https://github.com/permgps/herdr-telegram-agents/blob/main/README.md)
- [Architecture and development](https://github.com/permgps/herdr-telegram-agents/blob/main/docs/development.md)
- [Herdr adapter](https://github.com/permgps/herdr-telegram-agents/blob/main/internal/adapters/herdr/gateway.go)
- [Agent identity](https://github.com/permgps/herdr-telegram-agents/blob/main/internal/domain/agent.go) and [topic persistence](https://github.com/permgps/herdr-telegram-agents/blob/main/internal/domain/mapping.go)
- Local `maestri`, `maestri-workspace` and `maestri-manager` skills, checked against the installed CLI diagnostic attempt.
- [Maestri Wire, official contract](https://www.themaestri.app/pt-br/docs/wire)
- [Maestri Guide](https://github.com/arthurspk/guiadomaestri) and its [documented import/export limitations](https://github.com/arthurspk/guiadomaestri/blob/main/docs/10-importar-e-exportar.md)
- [Telegram forum topics](https://core.telegram.org/api/forum)

Sources consulted on September 13, 2026. Upstream history and licensing are preserved. Maestri integration and the catalog have been implemented; current limitations and outstanding live validation are described in the [setup guide](maestri-relay-setup.md).
