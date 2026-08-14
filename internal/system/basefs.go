package system

import (
	"fmt"
	"sort"
	"strings"

	"github.com/redlab/redlab/internal/scenario"
)

type baseDirectory struct {
	path  string
	owner string
	group string
	mode  uint32
}

// installRHELBase lays down the ordinary hierarchy an administrator expects
// to find on a minimal RHEL 8 host. These are virtual records only; none of
// the paths are mapped to the computer running RedLab.
func (s *State) installRHELBase(spec scenario.ScenarioSpec) {
	directories := []baseDirectory{
		{"/", "root", "root", 0755},
		{"/bin", "root", "root", 0755}, {"/boot", "root", "root", 0700},
		{"/dev", "root", "root", 0755}, {"/dev/pts", "root", "root", 0755}, {"/dev/shm", "root", "root", 01777},
		{"/etc", "root", "root", 0755}, {"/etc/cron.d", "root", "root", 0755}, {"/etc/cron.daily", "root", "root", 0755},
		{"/etc/dnf", "root", "root", 0755}, {"/etc/logrotate.d", "root", "root", 0755}, {"/etc/NetworkManager", "root", "root", 0755},
		{"/etc/NetworkManager/system-connections", "root", "root", 0700}, {"/etc/pki", "root", "root", 0755}, {"/etc/pki/ca-trust", "root", "root", 0755},
		{"/etc/security", "root", "root", 0755}, {"/etc/selinux", "root", "root", 0755}, {"/etc/selinux/targeted", "root", "root", 0755},
		{"/etc/ssh", "root", "root", 0755}, {"/etc/sudoers.d", "root", "root", 0750}, {"/etc/sysconfig", "root", "root", 0755},
		{"/etc/systemd", "root", "root", 0755}, {"/etc/systemd/system", "root", "root", 0755}, {"/etc/yum.repos.d", "root", "root", 0755},
		{"/home", "root", "root", 0755}, {"/media", "root", "root", 0755}, {"/mnt", "root", "root", 0755}, {"/opt", "root", "root", 0755},
		{"/proc", "root", "root", 0555}, {"/proc/1", "root", "root", 0555},
		{"/root", "root", "root", 0700}, {"/run", "root", "root", 0755}, {"/run/lock", "root", "root", 0775}, {"/run/user", "root", "root", 0755},
		{"/sbin", "root", "root", 0755}, {"/srv", "root", "root", 0755}, {"/sys", "root", "root", 0555},
		{"/tmp", "root", "root", 01777}, {"/usr", "root", "root", 0755}, {"/usr/bin", "root", "root", 0755},
		{"/usr/lib", "root", "root", 0755}, {"/usr/lib64", "root", "root", 0755}, {"/usr/lib/systemd", "root", "root", 0755},
		{"/usr/lib/systemd/system", "root", "root", 0755}, {"/usr/local", "root", "root", 0755}, {"/usr/local/bin", "root", "root", 0755},
		{"/usr/local/etc", "root", "root", 0755}, {"/usr/local/lib", "root", "root", 0755}, {"/usr/local/sbin", "root", "root", 0755},
		{"/usr/local/share", "root", "root", 0755}, {"/usr/sbin", "root", "root", 0755}, {"/usr/share", "root", "root", 0755},
		{"/usr/share/doc", "root", "root", 0755}, {"/usr/share/man", "root", "root", 0755}, {"/var", "root", "root", 0755},
		{"/var/cache", "root", "root", 0755}, {"/var/lib", "root", "root", 0755}, {"/var/lib/dnf", "root", "root", 0755},
		{"/var/lib/rpm", "root", "root", 0755}, {"/var/lib/systemd", "root", "root", 0755}, {"/var/log", "root", "root", 0755},
		{"/var/log/audit", "root", "root", 0700}, {"/var/log/journal", "root", "systemd-journal", 02755}, {"/var/opt", "root", "root", 0755},
		{"/var/spool", "root", "root", 0755}, {"/var/spool/cron", "root", "root", 0700}, {"/var/tmp", "root", "root", 01777},
	}
	for _, dir := range directories {
		s.Files[dir.path] = File{Path: dir.path, Owner: dir.owner, Group: dir.group, Mode: dir.mode, Directory: true, SELinuxType: directorySELinuxType(dir.path)}
	}
	executables := []string{
		"/sbin/init", "/usr/bin/awk", "/usr/bin/bash", "/usr/bin/basename", "/usr/bin/cat", "/usr/bin/chmod",
		"/usr/bin/chown", "/usr/bin/cp", "/usr/bin/cut", "/usr/bin/date", "/usr/bin/df", "/usr/bin/dirname",
		"/usr/bin/dnf", "/usr/bin/du", "/usr/bin/echo", "/usr/bin/find", "/usr/bin/firewall-cmd", "/usr/bin/free",
		"/usr/bin/grep", "/usr/bin/head", "/usr/bin/hostname", "/usr/bin/id", "/usr/bin/journalctl", "/usr/bin/kill",
		"/usr/bin/ls", "/usr/bin/mkdir", "/usr/bin/mv", "/usr/bin/pgrep", "/usr/bin/pkill", "/usr/bin/printf",
		"/usr/bin/ps", "/usr/bin/pstree", "/usr/bin/pwd", "/usr/bin/realpath", "/usr/bin/rm", "/usr/bin/rmdir",
		"/usr/bin/rpm", "/usr/bin/sed", "/usr/bin/sh", "/usr/bin/sort", "/usr/bin/ss", "/usr/bin/stat",
		"/usr/bin/sudo", "/usr/bin/systemctl", "/usr/bin/tail", "/usr/bin/tee", "/usr/bin/top", "/usr/bin/touch",
		"/usr/bin/tree", "/usr/bin/uname", "/usr/bin/uniq", "/usr/bin/uptime", "/usr/bin/wc", "/usr/bin/whoami",
		"/usr/sbin/groupadd", "/usr/sbin/httpd", "/usr/sbin/restorecon", "/usr/sbin/setenforce", "/usr/sbin/sshd",
		"/usr/sbin/useradd", "/usr/sbin/usermod",
	}
	for _, name := range executables {
		s.Files[name] = File{Path: name, Owner: "root", Group: "root", Mode: 0755, SELinuxType: "bin_t"}
	}
	for _, service := range spec.Services {
		unitName := service.Name
		if !strings.Contains(unitName, ".") {
			unitName += ".service"
		}
		name := "/usr/lib/systemd/system/" + unitName
		content := fmt.Sprintf("[Unit]\nDescription=%s\nAfter=network.target\n\n[Service]\nType=simple\nExecStart=/usr/sbin/%s\n\n[Install]\nWantedBy=multi-user.target\n", unitName, strings.TrimSuffix(unitName, ".service"))
		s.Files[name] = File{Path: name, Content: content, Owner: "root", Group: "root", Mode: 0644, SELinuxType: "systemd_unit_file_t"}
	}

	for _, user := range s.Users {
		home := userHome(user.Name)
		mode := uint32(0700)
		if user.Name != "root" {
			mode = 0750
		}
		s.Files[home] = File{Path: home, Owner: user.Name, Group: s.primaryGroupLocked(user.Name), Mode: mode, Directory: true, SELinuxType: "user_home_dir_t"}
		group := s.primaryGroupLocked(user.Name)
		s.Files[home+"/.bash_profile"] = File{Path: home + "/.bash_profile", Content: "# .bash_profile\nif [ -f ~/.bashrc ]; then . ~/.bashrc; fi\nPATH=$PATH:$HOME/.local/bin:$HOME/bin\n", Owner: user.Name, Group: group, Mode: 0644, SELinuxType: "user_home_t"}
		s.Files[home+"/.bashrc"] = File{Path: home + "/.bashrc", Content: "# .bashrc\nif [ -f /etc/bashrc ]; then . /etc/bashrc; fi\n", Owner: user.Name, Group: group, Mode: 0644, SELinuxType: "user_home_t"}
	}

	hostname := s.Hostname
	if hostname == "" {
		hostname = "localhost.localdomain"
		s.Hostname = hostname
	}
	major := spec.RHEL.Major
	if major == 0 {
		major = 8
	}
	profile := spec.RHEL.MinorProfile
	if profile == "" {
		profile = fmt.Sprintf("%d.10", major)
	}
	files := map[string]struct {
		content string
		mode    uint32
	}{
		"/etc/almalinux-release":    {fmt.Sprintf("RedLab Enterprise Linux release %s (Ootpa)\n", profile), 0644},
		"/etc/bashrc":               {"# System-wide functions and aliases for the RedLab shell\n", 0644},
		"/etc/fstab":                {"# Created by RedLab\n/dev/mapper/rhel-root / xfs defaults 0 0\nUUID=REDLAB-BOOT /boot xfs defaults 0 0\n", 0644},
		"/etc/group":                {renderGroup(s.Users), 0644},
		"/etc/hostname":             {hostname + "\n", 0644},
		"/etc/hosts":                {renderHosts(hostname, spec.Network), 0644},
		"/etc/issue":                {fmt.Sprintf("RedLab Enterprise Linux %s\nKernel \\r on an \\m\n", profile), 0644},
		"/etc/motd":                 {"This is a deterministic RedLab RHEL-compatible training host.\n", 0644},
		"/etc/os-release":           {renderOSRelease(profile, major), 0644},
		"/etc/passwd":               {renderPasswd(s.Users), 0644},
		"/etc/profile":              {"# System-wide environment for the RedLab shell\nPATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n", 0644},
		"/etc/redhat-release":       {fmt.Sprintf("RedLab Enterprise Linux release %s (Ootpa)\n", profile), 0644},
		"/etc/resolv.conf":          {renderResolv(spec.Network.DNS), 0644},
		"/etc/shells":               {"/bin/sh\n/bin/bash\n/usr/bin/sh\n/usr/bin/bash\n", 0644},
		"/etc/sudoers":              {"Defaults    !visiblepw\nDefaults    always_set_home\nroot ALL=(ALL) ALL\n%wheel ALL=(ALL) ALL\n", 0440},
		"/proc/1/cmdline":           {"/sbin/init\x00", 0444},
		"/proc/cpuinfo":             {"processor\t: 0\nmodel name\t: RedLab Virtual CPU\ncpu cores\t: 2\n", 0444},
		"/proc/filesystems":         {"nodev\tsysfs\nnodev\ttmpfs\nnodev\tproc\n\txfs\n", 0444},
		"/proc/loadavg":             {"0.00 0.01 0.05 1/96 1\n", 0444},
		"/proc/meminfo":             {"MemTotal:        2097152 kB\nMemFree:         1572864 kB\nMemAvailable:    1769472 kB\nBuffers:           32768 kB\nCached:           196608 kB\nSwapTotal:       1048576 kB\nSwapFree:        1048576 kB\n", 0444},
		"/proc/mounts":              {"/dev/mapper/rhel-root / xfs rw,relatime 0 0\nproc /proc proc rw,nosuid,nodev,noexec,relatime 0 0\nsysfs /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0\ntmpfs /run tmpfs rw,nosuid,nodev 0 0\n", 0444},
		"/proc/sys/kernel/hostname": {hostname + "\n", 0644},
		"/proc/uptime":              {"86400.00 172000.00\n", 0444},
		"/proc/version":             {"Linux version 4.18.0-553.el8_10.x86_64 (RedLab) #1 SMP x86_64 GNU/Linux\n", 0444},
		"/var/log/dnf.log":          {"", 0644},
		"/var/log/messages":         {fmt.Sprintf("%s systemd[1]: Started RedLab virtual system.\n", s.Clock.Format("Jan 02 15:04:05")), 0640},
		"/var/log/secure":           {"", 0600},
	}
	for name, entry := range files {
		s.ensureParents(name)
		s.Files[name] = File{Path: name, Content: entry.content, Owner: "root", Group: "root", Mode: entry.mode, SELinuxType: fileSELinuxType(name)}
	}
}

func userHome(name string) string {
	if name == "root" {
		return "/root"
	}
	return "/home/" + name
}

func renderOSRelease(profile string, major int) string {
	return fmt.Sprintf("NAME=\"RedLab Enterprise Linux\"\nVERSION=\"%s (Ootpa)\"\nID=\"rhel\"\nID_LIKE=\"fedora\"\nVERSION_ID=\"%s\"\nPLATFORM_ID=\"platform:el%d\"\nPRETTY_NAME=\"RedLab Enterprise Linux %s (Ootpa)\"\n", profile, profile, major, profile)
}

func renderPasswd(users map[string]User) string {
	ordered := sortedUsers(users)
	var b strings.Builder
	for _, user := range ordered {
		home := userHome(user.Name)
		fmt.Fprintf(&b, "%s:x:%d:%d:%s:%s:%s\n", user.Name, user.UID, user.GID, user.Name, home, defaultString(user.Shell, "/bin/bash"))
	}
	return b.String()
}

func renderGroup(users map[string]User) string {
	groups := map[string][]string{"root": {}}
	for _, user := range users {
		primary := user.Name
		groups[primary] = appendUnique(groups[primary], user.Name)
		for _, group := range user.Groups {
			groups[group] = appendUnique(groups[group], user.Name)
		}
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for i, name := range names {
		gid := 1000 + i
		if name == "root" {
			gid = 0
		} else if user, ok := users[name]; ok {
			gid = user.GID
		}
		fmt.Fprintf(&b, "%s:x:%d:%s\n", name, gid, strings.Join(groups[name], ","))
	}
	return b.String()
}

func (s *State) syncIdentityFilesLocked() {
	if file, ok := s.Files["/etc/passwd"]; ok {
		file.Content = renderPasswd(s.Users)
		s.Files[file.Path] = file
	}
	if file, ok := s.Files["/etc/group"]; ok {
		file.Content = renderGroup(s.Users)
		s.Files[file.Path] = file
	}
}

func sortedUsers(users map[string]User) []User {
	out := make([]User, 0, len(users))
	for _, user := range users {
		out = append(out, user)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UID < out[j].UID || out[i].UID == out[j].UID && out[i].Name < out[j].Name
	})
	return out
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func renderHosts(hostname string, network scenario.NetworkSpec) string {
	short := strings.SplitN(hostname, ".", 2)[0]
	var b strings.Builder
	fmt.Fprintf(&b, "127.0.0.1   localhost localhost.localdomain\n::1         localhost localhost.localdomain\n127.0.1.1   %s %s\n", hostname, short)
	names := make([]string, 0, len(network.DNS.Records))
	for name := range network.DNS.Records {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&b, "%s   %s\n", network.DNS.Records[name], name)
	}
	return b.String()
}

func renderResolv(dns scenario.DNSSpec) string {
	var b strings.Builder
	for _, server := range dns.Servers {
		fmt.Fprintf(&b, "nameserver %s\n", server)
	}
	if b.Len() == 0 {
		b.WriteString("nameserver 127.0.0.1\n")
	}
	return b.String()
}

func directorySELinuxType(name string) string {
	switch {
	case name == "/tmp" || name == "/var/tmp":
		return "tmp_t"
	case strings.HasPrefix(name, "/home/") || name == "/root":
		return "user_home_dir_t"
	case strings.HasPrefix(name, "/var/log"):
		return "var_log_t"
	case strings.HasPrefix(name, "/etc"):
		return "etc_t"
	default:
		return "root_t"
	}
}

func fileSELinuxType(name string) string {
	switch {
	case strings.HasPrefix(name, "/etc"):
		return "etc_t"
	case strings.HasPrefix(name, "/proc"):
		return "proc_t"
	case strings.HasPrefix(name, "/var/log"):
		return "var_log_t"
	default:
		return "default_t"
	}
}
