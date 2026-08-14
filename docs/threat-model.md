# RedLab threat model

## Assets

- Organizer host filesystem, processes, network, and credentials.
- Team join codes, judge credentials, signing keys, and event database.
- Scenario package integrity and submission evidence.
- Fairness and determinism of team sessions.

## Threats and controls

| Threat | Control |
| --- | --- |
| Participant command escapes into the host | No host command execution; command implementations receive virtual services only. |
| Participant reads host files or environment | Virtual filesystem and injected environment are separate from `os` state. |
| Emulated networking scans the LAN | Network commands resolve only scenario-defined hosts and endpoints. |
| Malicious scenario archive | Reject traversal, absolute paths, symlinks, duplicate entries, oversized files, and archive expansion over limits. |
| YAML executes code | Strict typed decoding and a registry of condition/effect names; no templates or scripting. |
| Evidence is altered after submission | Chained SHA-256 events and Ed25519-signed manifests. |
| Role bypass | Authentication and authorization are enforced at the handler and service layers. |
| Timing or random behavior changes replay | Virtual clock and recorded per-session seed are injected into the emulator. |
| Denial of service | Request, frame, output, transcript, scenario, and session limits plus server timeouts. |

## Out of scope

RedLab does not attempt to sandbox arbitrary native code. It therefore does not execute participant binaries, real containers, real exploits, host shell scripts, or arbitrary plugins.
