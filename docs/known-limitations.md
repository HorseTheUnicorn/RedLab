# Known limitations

- The shell is a bounded interactive subset. Full Bash scripts, command substitution, job control, and arbitrary shell functions are intentionally unsupported.
- The full v1 catalog is classified, but many depth-oriented commands are intentionally Level C. Level C commands return an explicit RedLab compatibility message and never execute a host command.
- Every scenario receives a populated RHEL-like directory tree and common system metadata, and participants may navigate and modify that virtual tree normally. Pseudo-files such as those under `/proc` are deterministic representations rather than kernel-backed live files, and scenario-specific mechanics remain intentionally smaller than a full RHEL installation.
- SQLite persistence rebuilds active sessions from the event log on server startup and persists authentication signing/refresh state; snapshot compaction and persistence of evidence signing keys remain future hardening work.
- Participants can work fully offline with `redlab play --export`; coordinated remote events remain an optional organizer-server mode. Offline bundle transfer and judging are intentionally out-of-band.
- LAN TLS uses an event-owned self-signed server certificate and fingerprint pinning in the join client; a full local CA hierarchy and certificate rotation workflow remain future hardening work.
- The organizer service is intended for localhost, a trusted event LAN, or a private VPN. Public-internet hosting and hostile multi-tenant isolation are outside the supported deployment model.
- The score engine supports automated state checks and guardrails; organizer rubric scores and notes are available through the authenticated organizer API and are included in reports. Signed post-submission override artifacts remain future hardening.
