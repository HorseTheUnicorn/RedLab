# ADR 0001: Behavioral emulator boundary

Status: accepted

RedLab models the system-administration surfaces needed by supported scenarios instead of running a VM, container, kernel, or host shell. This makes local practice and a single-process LAN event practical while keeping participant commands away from organizer resources. Unsupported behavior is cataloged and returns a realistic RedLab error.
