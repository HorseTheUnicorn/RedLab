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
| Malicious scenario archive | Extract only into a new directory through a rooted filesystem handle; reject traversal, absolute paths, symlinks, duplicate/colliding entries, oversized files, and archive expansion over limits. |
| Malicious evidence bundle | Require canonical entry names, bounded sizes/counts, a signed manifest hash for every payload, and no unsigned extra entries. |
| YAML executes code | Strict typed decoding and a registry of condition/effect names; no templates or scripting. |
| Evidence is altered after submission | Chained SHA-256 events and Ed25519-signed manifests. |
| Role bypass | Authentication fails closed when the credential store is absent or invalid; authorization is enforced at the handler and service layers. Credentials use bcrypt, with automatic migration after a successful legacy login. |
| Credentials cross an untrusted network | The client refuses plaintext HTTP except on loopback. LAN events use TLS and explicit SHA-256 certificate fingerprint pinning. |
| Scenario passwords leak in exports | Scenario-defined passwords are redacted from transcripts, reports, and exported evidence while raw recovery events remain local to the event database. Never use real credentials in a scenario. |
| Accidental LAN exposure | `serve` forces loopback unless the organizer explicitly supplies `--lan`. |
| Timing or random behavior changes replay | Virtual clock and recorded per-session seed are injected into the emulator. |
| Denial of service | Request, authentication, multipart body, frame, output, transcript, scenario, bundle, and session limits plus server timeouts. |

## Out of scope

RedLab does not attempt to sandbox arbitrary native code. It therefore does not execute participant binaries, real containers, real exploits, host shell scripts, or arbitrary plugins.

The organizer service is designed for a trusted hackathon LAN, VPN, or localhost—not direct exposure to the public internet. Protect the event directory with the organizer's operating-system account, distribute join codes out of band, and do not place real production secrets in event or scenario files.
