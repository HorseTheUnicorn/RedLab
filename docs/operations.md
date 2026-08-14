# Event-day operations

1. Run `redlab event init ./event` and keep the printed organizer recovery secret and team join code out of chat transcripts.
2. Add enabled scenario package paths to `event.yaml` and run `redlab event validate ./event/event.yaml`.
3. Run `redlab scenario validate` and `redlab scenario test` for each reference scenario before opening the event.
4. For a serverless event, distribute the scenario packages and have each participant run `redlab play ... --export <bundle>` locally. Collect the signed bundles through the event's approved file-transfer channel.
5. For a coordinated remote event, use `redlab serve --event ./event/event.yaml`; use `--lan` only when TLS is configured. Give each team the URL, join code, and generated certificate fingerprint for `redlab join`.
6. Review offline bundles with `redlab submissions list <directory>`, `redlab evidence verify <bundle>`, and `redlab judge <bundle>`. In server mode, bundles are also under the event-owned `data/submissions` directory.

The server does not change the organizer firewall. Configure the LAN firewall and verify the printed bind address separately.
