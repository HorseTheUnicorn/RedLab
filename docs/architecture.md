# RedLab architecture

## Boundary

RedLab emulates selected RHEL 8 behavior. The virtual filesystem, users, services, clock, network, firewall, packages, and SELinux state are ordinary Go data structures. Participant commands receive interfaces to those structures and cannot call `os/exec`, inspect host process state, read the host environment, or open outbound sockets.

The organizer backend may use the host filesystem for event configuration, immutable scenario packages, SQLite persistence, and exported reports. That host access is outside the participant command path and is constrained by package validation and path-safe readers.

## Components

1. `scenario`: strict YAML documents, semantic validation, and safe package inspection.
2. `system`: deterministic virtual RHEL state and typed state mutations.
3. `shell`: bounded lexer/parser with pipes, redirects, sequencing, and expansion.
4. `command`: compatibility catalog and injected command implementations.
5. `rules` and `scoring`: declarative conditions, effects, objectives, hints, and guardrails.
6. `evidence` and `report`: append-only hash chains, redaction, and Markdown/JSON exports.
7. `server` and `store`: organizer API, session serialization, and SQLite persistence.

Each active session has one mutation owner. State is initialized from an immutable scenario snapshot and can be replayed from evidence events.

## Decisions

- YAML is data plus registered condition/effect values, never executable code.
- Compatibility is explicit: scenario-grade, common inspection, recognized, or unavailable.
- `redlab serve` binds loopback by default. LAN mode is an explicit opt-in and requires TLS configuration.
- Virtual paths are POSIX paths kept in memory or serialized into RedLab-owned data files; they are never translated into host paths.
