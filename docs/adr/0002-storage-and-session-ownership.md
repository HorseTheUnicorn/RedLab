# ADR 0002: SQLite persistence and serialized session ownership

Status: accepted

SQLite is the source of truth for event metadata, users, sessions, evidence, snapshots, and submissions. Each active session has one coordinator that serializes state mutations. API handlers enqueue work and never mutate virtual state directly, which makes replay, race testing, and reconnect behavior tractable.
