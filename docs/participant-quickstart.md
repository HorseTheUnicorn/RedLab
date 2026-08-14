# Participant quick start

For a local/offline submission, no server is needed:

```text
redlab play ./scenario-packs/core/broken-httpd --team TEAM-1 --export ./TEAM-1-submission.rlab.zip
```

Run `lab submit` before `exit`; send the resulting signed bundle to the organizer through the event's chosen file-transfer channel.

For an optional coordinated remote event, connect to the organizer backend:

```text
redlab join https://organizer:8443 --team TEAM-1 --join-code CODE --trust-fingerprint FINGERPRINT
```

Inside the terminal, begin with `lab briefing`, `pwd`, `ls -la`, `id`, `uname -a`, `systemctl status <unit>`, `journalctl`, and the relevant inspection commands. The virtual host has the usual RHEL-style paths, so you can explore `/etc`, `/home`, `/proc`, `/run`, `/usr`, and `/var` with `cd`, `find`, `tree`, and related commands. Use `lab check` to re-evaluate state, `lab note` and `lab answer` to preserve your explanation, then `lab evidence` and `lab submit`.

All paths, processes, network hosts, and time shown by the terminal are virtual scenario state. Participant commands never execute on the participant's host.
