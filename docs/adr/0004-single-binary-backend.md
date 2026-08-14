# ADR 0004: Single-binary embedded backend

Status: accepted

The organizer backend is embedded in the `redlab` executable. Loopback practice requires no permanent server. LAN mode is explicit, uses TLS, and reports a certificate fingerprint; the application never changes the organizer's firewall automatically.
