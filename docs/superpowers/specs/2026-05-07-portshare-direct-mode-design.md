# portshare Direct Mode Design

## Background

The current MVP proves that a local HTTP service can be exposed inside a tailnet with Tailscale Serve. That validates the basic networking environment, but it is not the desired product shape. The product goal is now direct device-to-device use: both computers run `portshare`, establish trust with the same shared secret, and then create local TCP forwarding entries on demand.

The application name is `portshare`. The name should not be translated in window titles, tray menus, documentation headings, or user-facing product references. The UI may remain Chinese by default.

## Goals

- Rename the visible product from `端口发布器` to `portshare`.
- Replace the main MVP flow with a device pairing and local TCP forwarding flow.
- Use Tailscale as the required private network substrate.
- Avoid Tailscale Serve and Funnel for the main direct mode.
- Avoid exposing business service ports through Tailscale Serve, Funnel, LAN, or public internet.
- Let paired devices access arbitrary TCP ports through a `portshare`-managed local forwarding entry.
- Diagnose Tailscale readiness and connection failures clearly enough that users know what to fix.

## Non-Goals

- UDP forwarding.
- Public internet sharing.
- Per-port authorization in the first direct-mode MVP.
- NAT traversal implemented by `portshare` itself.
- Replacing Tailscale or running without Tailscale.
- Transparent OS-level routing where every remote port appears automatically without a local forwarding entry.

## Direct Mode Definition

Direct mode means two `portshare` clients connect to each other over Tailscale IPs and authenticate at the application layer with a shared secret. Business traffic goes through `portshare` on both machines:

```text
local app/browser
  -> 127.0.0.1:<local-forward-port>
  -> local portshare
  -> peer Tailscale IP:17890
  -> peer portshare
  -> peer local TCP target, for example 127.0.0.1:3000
```

The underlying Tailscale path may be peer-to-peer UDP or DERP depending on the network. `portshare` should surface this information with `tailscale ping`, but it does not implement its own NAT traversal or force DERP avoidance.

## Tailscale Dependency

`portshare` must actively inspect and use Tailscale capabilities before pairing or forwarding.

Startup checks:

- `tailscale` CLI is available.
- Tailscale backend is running.
- The device is logged in.
- The device has at least one Tailscale IPv4 address.
- The app can bind its control listener to the local Tailscale IP.

Peer checks:

- `tailscale ping <peer-ip>` succeeds.
- The UI shows whether the current path is direct or DERP and includes latency.
- The peer control endpoint `<peer-ip>:17890` is reachable.
- If a user enters a DNS name, MagicDNS resolution is checked. If DNS is not accepted locally, show a fix for `tailscale set --accept-dns=true` or ask the user to use the Tailscale IP.

Troubleshooting checks:

- Detect `accept-dns=false` when name resolution fails.
- Detect or warn about Shields Up when incoming control connections appear blocked.
- Distinguish common failure layers:
  - Tailscale not installed.
  - Tailscale not running.
  - Not logged in.
  - No Tailscale IP.
  - Peer offline or unreachable.
  - Peer `portshare` not running.
  - Shared secret mismatch.
  - Local control port bind failure.
  - Local forwarding port already in use.
  - Peer target port refused or timed out.

Safe automatic fixes may be exposed as buttons or explicit actions:

- `tailscale set --accept-dns=true`
- `tailscale set --shields-up=false`

Potentially disruptive operations, such as login or changing broader Tailscale settings, should be presented as guided manual steps instead of being run silently.

## Control Listener

Each running `portshare` instance starts a control listener on a fixed default port:

```text
<local-tailscale-ip>:17890
```

The listener must not bind `0.0.0.0` for direct mode. It should bind the Tailscale IP selected from the local Tailscale status. If binding fails, the UI should show a diagnostic message and direct mode should not be marked ready.

The control port is an internal `portshare` endpoint. It is not a business service port. It accepts only authenticated `portshare` protocol requests.

## Pairing Model

Pairing is device-level trust, not port-level trust.

User flow:

1. Both computers open `portshare`.
2. Both computers enter the same shared secret.
3. One side enters the other side's Tailscale IP.
4. The initiating client connects to `<peer-tailscale-ip>:17890`.
5. The two clients perform a challenge-response handshake using the shared secret.
6. If the handshake succeeds, both sides treat the peer as trusted.
7. The trusted peer is saved locally.

The shared secret must not be sent in clear text. MVP authentication should use a nonce-based HMAC-SHA256 challenge-response:

- Initiator sends protocol version, local device identity, and a random nonce.
- Responder replies with its own nonce and HMAC proof.
- Initiator verifies the responder proof and sends its own HMAC proof.
- Responder verifies the initiator proof.
- Both sides derive a session key from the shared secret and nonces for request authentication during this connection.

The MVP may use the shared secret for each new session instead of implementing long-term public keys, but stored pairing records must not contain the plain shared secret.

Stored pairing records include:

- Peer Tailscale IP.
- Peer display name if available.
- First paired time.
- Last connected time.
- Last observed route type, such as direct or DERP.
- A local pairing identifier derived from the secret, not the secret itself.

## Forwarding Model

After pairing, either side can create local TCP forwarding entries to the peer.

Forward creation inputs:

- Trusted peer.
- Peer target host and port, defaulting to `127.0.0.1:<remote-port>`.
- Local listen host, defaulting to `127.0.0.1`.
- Local listen port, either user-specified or automatically allocated.

Example:

```text
127.0.0.1:18080 -> 100.109.251.97:17890 -> 127.0.0.1:3000
```

Creating a forward does not require a new pairing authorization. It does require that the peer remains reachable and authenticates with the existing shared-secret relationship.

Forwarding supports arbitrary TCP streams. It should not assume HTTP semantics and must work for SSH, databases, development servers, and other TCP protocols.

Stopping a forward closes the local listener and active streams for that forward. It does not remove the trusted device pairing.

Removing a pairing stops all forwards associated with that peer.

## UI Requirements

The main UI should be centered on direct mode:

- Product title: `portshare`.
- Local Tailscale status panel:
  - Running or not running.
  - Local Tailscale IP.
  - Control listener status.
  - DNS readiness if relevant.
- Shared secret input:
  - Enter or update the current secret.
  - Show whether direct mode is ready.
- Pair peer panel:
  - Peer Tailscale IP or name input.
  - Connect/pair action.
  - Pairing progress and diagnostics.
- Trusted devices list:
  - Device name/IP.
  - Last connected.
  - Route type and latency from the latest `tailscale ping`.
  - Remove pairing action.
- Forwarding panel:
  - Select trusted peer.
  - Remote host/port.
  - Local port.
  - Create forward.
  - Running forwards with local URL/address and stop action.

Existing service discovery and Tailscale Serve buttons may remain behind an advanced or legacy section during transition, but they are not the main MVP flow.

## Security UX

The UI must clearly state:

- A paired device can request access to arbitrary local TCP ports.
- Pair only with a computer you control or explicitly trust.
- The shared secret is used for pairing and should be treated like a password.
- Business ports are not exposed directly through Tailscale Serve or Funnel in direct mode.

Failed authentication attempts should be recorded in the audit log with the peer IP and reason, but not with the shared secret.

## Provider Architecture

The existing provider abstraction is oriented around publishing services. Direct mode needs a separate subsystem rather than forcing device pairing into the publish provider shape.

New internal areas should be introduced:

- `internal/tailscale`: local Tailscale CLI/status/diagnostic adapter.
- `internal/direct/protocol`: handshake and stream protocol.
- `internal/direct/server`: control listener and peer request handling.
- `internal/direct/client`: peer dialing and authenticated session creation.
- `internal/direct/forward`: local TCP listener and bidirectional stream piping.
- `internal/direct/store`: trusted peer persistence.
- `internal/direct/manager`: orchestration for readiness, pairing, forwards, diagnostics, and audit events.

The older `internal/provider/tailscale` can remain for legacy Serve/Funnel behavior, but direct mode should not call `tailscale serve`.

## Protocol Sketch

The protocol runs over TCP on the peer control port.

Control messages are length-prefixed JSON for the handshake and forwarding setup. TCP payload streams use raw byte piping after a forward request is accepted.

Minimum message types:

- `hello`
- `hello_response`
- `auth_proof`
- `auth_ok`
- `open_tcp`
- `open_tcp_ok`
- `open_tcp_error`

An `open_tcp` request contains the peer-local target host and port. After `open_tcp_ok`, both sides pipe raw bytes until either side closes.

All control messages include a protocol version. Incompatible versions fail with a clear error.

## Testing Strategy

Unit tests:

- HMAC challenge-response succeeds with matching secrets.
- HMAC challenge-response fails with mismatched secrets.
- Stored peer records never contain the plain shared secret.
- Tailscale diagnostics classify missing CLI, stopped backend, no IP, DNS not accepted, and peer unreachable states.
- Forward manager rejects local port conflicts.
- Forward manager can pipe bytes through a fake peer session.

Integration tests with loopback:

- Start two direct servers on loopback-only test addresses and different control ports.
- Pair with a shared secret.
- Create a local forward from one test instance to the other.
- Verify an HTTP test server behind the peer is reachable through the local forward.
- Verify stopping the forward makes the local entry unreachable.

Manual verification:

- Run `portshare` on two Windows computers in the same tailnet.
- Confirm both show Tailscale ready and control listener ready.
- Enter the same shared secret on both computers.
- Pair by entering the peer Tailscale IP.
- Confirm route diagnostics show direct or DERP with latency.
- Create a local forward to the peer's `127.0.0.1:3000`.
- Access `http://127.0.0.1:<local-port>/` from the initiating computer.
- Stop the forward and confirm access fails.
- Turn off DNS acceptance on one machine and confirm diagnostics explain the issue if a DNS name is used.

## Migration From Current MVP

Short-term:

- Keep current tests passing while adding direct-mode modules.
- Rename visible product references to `portshare`.
- Move Tailscale Serve/Funnel actions out of the main path.

Future:

- Decide whether to remove legacy Serve/Funnel from the product or keep it as an advanced sharing mode.
- Add stronger long-term device keys after initial shared-secret MVP.
- Add per-peer or per-port access policies if device-level trust is too broad for real users.
