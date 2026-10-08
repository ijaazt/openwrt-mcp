# Operation cards

This fork embeds one self-contained MCP Apps resource in the Go executable:
`ui://openwrt/operation-card-v3.html`. All nine tools advertise it through standard
`_meta.ui.resourceUri` and ChatGPT output-template metadata. Successful, denied and
handler-error results include a structured `card` payload and preserve upstream
text content. Protocol-level validation errors can occur before a handler runs.

| Operation | Card |
| --- | --- |
| `ubus_list` | Discovered objects and methods |
| `ubus_call` | Board, memory, network, DHCP or generic structured results |
| `uci_get` | Configuration settings |
| `uci_apply` | Requested changes, deadline, inspection control |
| `uci_confirm` | Confirmation result |
| `logread` | Log severity totals and entries |
| `exec` | Command output or permission denial |
| `wg_new_client` | Creation result without duplicating private configuration/QR |
| `mfa_unlock` | Access result without authentication code |

Refresh is offered only for known diagnostic reads. Generic `ubus_call` retains
write annotations because the operation can mutate state. No card invokes a tool
on startup; changes and confirmations cannot be replayed by card controls.

Card data uses DOM text nodes, a restrictive resource/connect CSP, and no external
assets. UCI credential options, JSON credential fields, confirmation tokens and
TOTP codes are redacted from structured card payloads. Existing text output is
preserved for upstream compatibility and can still contain sensitive credentials;
treat it accordingly. The upstream policy remains the authority for every call.

`show_operation_card` is an additional read-only viewer for the most recent sanitized
result of an operation. It preserves the original outcome, but retrieval itself
succeeds so hosts that hide tool-error widgets can show the denial/error card.
Results are isolated by authenticated client and expire after five minutes or a
server restart. It never repeats an operation.

A rollback countdown describes a deadline, not a verified rollback. Inspect after
it expires. Upstream snapshots live in `/tmp`: daemon restarts preserve them, but a
router reboot/power loss does not. UCI changes support scalar options only.

## Build and verify

The generated `ui/card.html` is checked in, so ordinary Go builds need no Node.
To change the UI:

```sh
npm ci
npm run check:ui
npm run build:ui
go vet ./...
go test -race ./...
python3 -m venv .venv
.venv/bin/pip install playwright
.venv/bin/playwright install chromium
.venv/bin/python scripts/verify-mobile.py
```

The browser verifier uses an isolated MCP Apps host with fixtures for all nine
operations plus errors and denials. It verifies the SDK handshake, escaping,
mobile overflow, theme changes, read actions and absence of mutation replay.
Set `CHROMIUM_PATH` to use a system browser. Screenshots go to ignored
`ui/test-results/`. Go tests also verify metadata, resource delivery, structured
results over a real SDK transport, redaction and safe-refresh classification.

Test representative cards in the actual client after deployment and refresh its
tool catalog if cached. Do not widen router permissions merely to exercise cards
for disabled operations. Router credentials, private deployment records and tunnel
configuration belong outside version control; `/deploy` is ignored in this fork.

Cards use MCP Apps host typography and color variables when available, with
ChatGPT-style neutral light/dark fallbacks. All read controls and output toggles
have at least 44-pixel touch targets. The mobile suite covers 320, 375, 390, 430
and 844-pixel viewports with touch emulation, both themes, expanded output,
read controls, host style changes and horizontal overflow checks (110 cases).

## Host integration

The server advertises inline as the available/preferred display mode. The card
requests no fullscreen mode and reports its content height through MCP Apps.
On initialization and each host-context change, it applies the host’s CSS color,
font and radius variables and theme. Layout uses responsive intrinsic sizes;
there are no selectors into ChatGPT’s private DOM or version-specific UI hooks.
Tests simulate changed host colors and typography to check that the card follows
them. Neutral fallbacks remain for hosts that omit style variables. Future host
breaking changes cannot be guaranteed; client integration still needs periodic
verification.
