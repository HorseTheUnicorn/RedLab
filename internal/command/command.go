package command

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/redlab/redlab/internal/catalog"
	"github.com/redlab/redlab/internal/system"
)

type Result struct {
	ExitCode  int
	Stdout    string
	Stderr    string
	Mutations []string
}
type Env struct {
	State     *system.State
	Variables map[string]string
	History   []string
	User      string
	CWD       string
	Lab       LabAPI
}
type LabAPI interface {
	RunLab(args []string, env *Env) Result
}
type Handler func(env *Env, args []string, stdin string) Result
type Command struct {
	Entry catalog.Entry
	Run   Handler
}
type Registry struct{ commands map[string]Command }

func NewRegistry() *Registry { return &Registry{commands: map[string]Command{}} }
func (r *Registry) Register(entry catalog.Entry, handler Handler) {
	r.commands[entry.Name] = Command{Entry: entry, Run: handler}
}
func (r *Registry) Lookup(name string) (Command, bool) { c, ok := r.commands[name]; return c, ok }
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.commands))
	for n := range r.commands {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
func (r *Registry) Run(name string, env *Env, args []string, stdin string) Result {
	if name == "lab" && env.Lab != nil {
		return env.Lab.RunLab(args, env)
	}
	c, ok := r.Lookup(name)
	if !ok {
		return Result{ExitCode: 127, Stderr: name + ": command not found\n"}
	}
	return c.Run(env, args, stdin)
}

func RegisterCore(r *Registry) {
	reg := func(name, pack, summary string, level catalog.Level, h Handler) {
		r.Register(catalog.Entry{Name: name, Pack: pack, Summary: summary, Level: level, Implemented: true}, h)
	}
	reg("pwd", "shell", "Print the virtual working directory", catalog.LevelA, func(e *Env, a []string, in string) Result { return Result{Stdout: e.CWD + "\n"} })
	reg("bash", "shell", "Enter the bounded RedLab shell", catalog.LevelA, runShellInfo)
	reg("sh", "shell", "Enter the bounded RedLab shell", catalog.LevelA, runShellInfo)
	reg("alias", "shell", "Inspect the bounded shell alias table", catalog.LevelB, runAlias)
	reg("help", "shell", "Show RedLab command help", catalog.LevelA, runHelp)
	reg("man", "shell", "Show a compatibility entry", catalog.LevelB, runMan)
	reg("clear", "shell", "Clear the virtual terminal", catalog.LevelA, func(_ *Env, _ []string, _ string) Result { return Result{Stdout: "\x1b[2J\x1b[H"} })
	reg("exit", "shell", "Leave the bounded shell", catalog.LevelA, func(_ *Env, _ []string, _ string) Result { return Result{Stdout: "logout\n"} })
	reg("cd", "shell", "Change the virtual working directory", catalog.LevelA, func(e *Env, a []string, in string) Result {
		target := e.Variables["HOME"]
		printTarget := false
		if len(a) > 0 {
			target = a[0]
		}
		if target == "-" {
			target = e.Variables["OLDPWD"]
			printTarget = true
			if target == "" {
				return Result{ExitCode: 1, Stderr: "cd: OLDPWD not set\n"}
			}
		}
		old := e.CWD
		if err := e.State.SetCWD(target); err != nil {
			return Result{ExitCode: 1, Stderr: err.Error() + "\n"}
		}
		e.CWD = e.State.CWD
		e.Variables["OLDPWD"] = old
		e.Variables["PWD"] = e.CWD
		if printTarget {
			return Result{Stdout: e.CWD + "\n"}
		}
		return Result{}
	})
	reg("echo", "shell", "Write arguments", catalog.LevelA, func(e *Env, a []string, in string) Result { return Result{Stdout: strings.Join(a, " ") + "\n"} })
	reg("printf", "shell", "Format text", catalog.LevelA, runPrintf)
	reg("date", "shell", "Print the deterministic virtual clock", catalog.LevelA, runDate)
	reg("which", "shell", "Resolve a command in the compatibility catalog", catalog.LevelA, runWhich)
	reg("type", "shell", "Describe a command in the compatibility catalog", catalog.LevelA, runWhich)
	reg("whereis", "shell", "Locate a command in the virtual image", catalog.LevelB, runWhich)
	reg("wc", "shell", "Count virtual text lines, words, and bytes", catalog.LevelB, runWC)
	reg("sort", "shell", "Sort virtual text lines", catalog.LevelB, runSort)
	reg("uniq", "shell", "Collapse adjacent duplicate lines", catalog.LevelB, runUniq)
	reg("cut", "shell", "Select delimited virtual text fields", catalog.LevelB, runCut)
	reg("env", "shell", "Print the injected environment", catalog.LevelA, func(e *Env, a []string, in string) Result {
		keys := make([]string, 0, len(e.Variables))
		for k := range e.Variables {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&b, "%s=%s\n", k, e.Variables[k])
		}
		return Result{Stdout: b.String()}
	})
	reg("export", "shell", "Set an injected environment variable", catalog.LevelA, func(e *Env, a []string, in string) Result {
		for _, item := range a {
			parts := strings.SplitN(item, "=", 2)
			if len(parts) != 2 {
				return Result{ExitCode: 1, Stderr: "export: invalid assignment\n"}
			}
			e.Variables[parts[0]] = parts[1]
		}
		return Result{}
	})
	reg("unset", "shell", "Remove an injected environment variable", catalog.LevelA, func(e *Env, a []string, in string) Result {
		for _, key := range a {
			delete(e.Variables, key)
		}
		return Result{}
	})
	reg("true", "shell", "Return success", catalog.LevelA, func(_ *Env, _ []string, _ string) Result { return Result{} })
	reg("false", "shell", "Return failure", catalog.LevelA, func(_ *Env, _ []string, _ string) Result { return Result{ExitCode: 1} })
	reg("test", "shell", "Evaluate basic virtual filesystem predicates", catalog.LevelA, runTest)
	reg("ls", "files", "List virtual filesystem entries", catalog.LevelA, runLS)
	reg("cat", "files", "Read virtual files", catalog.LevelA, runCat)
	reg("head", "files", "Read first lines", catalog.LevelB, runHead)
	reg("tail", "files", "Read last lines", catalog.LevelB, runTail)
	reg("grep", "files", "Search virtual files or stdin", catalog.LevelA, runGrep)
	reg("tee", "files", "Write stdin to virtual files and stdout", catalog.LevelA, runTee)
	reg("stat", "files", "Inspect virtual metadata", catalog.LevelB, runStat)
	reg("mkdir", "files", "Create virtual directory", catalog.LevelA, runMkdir)
	reg("touch", "files", "Create virtual file", catalog.LevelA, runTouch)
	reg("rm", "files", "Remove virtual entries", catalog.LevelA, runRM)
	reg("chmod", "files", "Change virtual mode bits", catalog.LevelA, runChmod)
	reg("chown", "files", "Change virtual ownership", catalog.LevelA, runChown)
	reg("cp", "files", "Copy a virtual file", catalog.LevelA, runCopy)
	reg("mv", "files", "Move a virtual file", catalog.LevelA, runMove)
	reg("rmdir", "files", "Remove an empty virtual directory", catalog.LevelB, runRmdir)
	reg("find", "files", "Find entries in the virtual filesystem", catalog.LevelB, runFind)
	reg("basename", "files", "Strip directory components from a virtual path", catalog.LevelA, runBaseName)
	reg("dirname", "files", "Strip the final component from a virtual path", catalog.LevelA, runDirName)
	reg("realpath", "files", "Resolve a canonical virtual path", catalog.LevelB, runRealPath)
	reg("tree", "files", "Display a virtual directory tree", catalog.LevelB, runTree)
	reg("whoami", "identity", "Print effective user", catalog.LevelA, func(e *Env, _ []string, _ string) Result { return Result{Stdout: e.User + "\n"} })
	reg("id", "identity", "Print virtual identity", catalog.LevelA, runID)
	reg("groups", "identity", "Print group membership", catalog.LevelA, runGroups)
	reg("sudo", "identity", "Run under simulated sudo policy", catalog.LevelA, runSudo)
	reg("usermod", "identity", "Modify virtual user group membership", catalog.LevelA, runUsermod)
	reg("useradd", "identity", "Create a virtual user", catalog.LevelB, runUseradd)
	reg("systemctl", "systemd", "Inspect and mutate virtual services", catalog.LevelA, runSystemctl)
	reg("journalctl", "systemd", "Inspect virtual journal", catalog.LevelA, runJournal)
	reg("logger", "systemd", "Append virtual journal entry", catalog.LevelA, runLogger)
	reg("hostname", "networking", "Print the virtual hostname", catalog.LevelA, runHostname)
	reg("host", "networking", "Resolve a scenario-defined virtual host", catalog.LevelB, runHost)
	reg("nslookup", "networking", "Resolve a scenario-defined virtual host", catalog.LevelB, runHost)
	reg("ip", "networking", "Inspect virtual interfaces", catalog.LevelB, runIP)
	reg("ss", "networking", "Inspect virtual sockets", catalog.LevelB, runSS)
	reg("ping", "networking", "Probe virtual host", catalog.LevelB, runPing)
	reg("dig", "networking", "Resolve virtual DNS", catalog.LevelB, runDig)
	reg("curl", "networking", "Request virtual HTTP endpoint", catalog.LevelA, runCurl)
	reg("firewall-cmd", "firewalld", "Inspect and mutate virtual firewall", catalog.LevelA, runFirewall)
	reg("getenforce", "selinux", "Print virtual SELinux mode", catalog.LevelA, func(e *Env, _ []string, _ string) Result { return Result{Stdout: e.State.SELinuxMode() + "\n"} })
	reg("setenforce", "selinux", "Change virtual SELinux mode", catalog.LevelA, runSetEnforce)
	reg("sestatus", "selinux", "Print virtual SELinux status", catalog.LevelA, runSEStatus)
	reg("restorecon", "selinux", "Restore virtual context", catalog.LevelA, runRestorecon)
	reg("rpm", "packages", "Inspect virtual installed packages", catalog.LevelA, runRPM)
	reg("dnf", "packages", "Inspect virtual package state", catalog.LevelB, runDNF)
	reg("ps", "processes", "List deterministic virtual processes", catalog.LevelB, runPS)
	reg("pstree", "processes", "Show the virtual process tree", catalog.LevelB, runPSTree)
	reg("pgrep", "processes", "Find virtual processes by command", catalog.LevelB, runPGrep)
	reg("pkill", "processes", "Signal virtual processes by command", catalog.LevelB, runPKill)
	reg("kill", "processes", "Signal a virtual process", catalog.LevelB, runKill)
	reg("killall", "processes", "Signal virtual processes by name", catalog.LevelB, runKillAll)
	reg("top", "processes", "Show a deterministic virtual process summary", catalog.LevelB, runTop)
	reg("df", "storage", "Report deterministic virtual filesystem usage", catalog.LevelB, runDF)
	reg("du", "storage", "Estimate virtual file space usage", catalog.LevelB, runDU)
	reg("free", "processes", "Display deterministic virtual memory usage", catalog.LevelB, runFree)
	reg("uptime", "processes", "Display deterministic virtual uptime and load", catalog.LevelB, runUptime)
	reg("uname", "shell", "Print virtual kernel information", catalog.LevelA, runUname)
	reg("history", "shell", "Print command history", catalog.LevelA, func(e *Env, _ []string, _ string) Result {
		var b strings.Builder
		for i, h := range e.History {
			fmt.Fprintf(&b, "%5d  %s\n", i+1, h)
		}
		return Result{Stdout: b.String()}
	})
	for _, entry := range catalog.Entries() {
		if _, exists := r.Lookup(entry.Name); exists {
			continue
		}
		captured := entry
		r.Register(captured, func(_ *Env, _ []string, _ string) Result {
			return Result{ExitCode: 127, Stderr: captured.Name + ": recognized by RedLab, but not emulated at compatibility level " + string(captured.Level) + "\n"}
		})
	}
}

func runPrintf(_ *Env, args []string, _ string) Result {
	if len(args) == 0 {
		return Result{}
	}
	format := args[0]
	values := make([]any, 0, len(args)-1)
	for _, v := range args[1:] {
		values = append(values, v)
	}
	return Result{Stdout: fmt.Sprintf(format, values...)}
}

func runShellInfo(_ *Env, args []string, _ string) Result {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--version" || args[0] == "-V")) {
		return Result{Stdout: "RedLab bounded shell (RHEL 8 compatibility)\n"}
	}
	return Result{ExitCode: 2, Stderr: "shell: interactive host execution is unavailable\n"}
}

func runAlias(_ *Env, args []string, _ string) Result {
	if len(args) > 0 {
		return Result{ExitCode: 1, Stderr: "alias: no aliases are configured in the bounded shell\n"}
	}
	return Result{Stdout: "alias: no aliases configured\n"}
}

func runHelp(_ *Env, args []string, _ string) Result {
	if len(args) > 1 {
		return Result{ExitCode: 2, Stderr: "help: accepts at most one command\n"}
	}
	if len(args) == 1 {
		entry, ok := catalog.Lookup(args[0])
		if !ok {
			return Result{ExitCode: 1, Stderr: "help: " + args[0] + ": command not found\n"}
		}
		return Result{Stdout: fmt.Sprintf("%-18s [%s/%s] %s\n", entry.Name, entry.Pack, entry.Level, entry.Summary)}
	}
	entries := catalog.Entries()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	var b strings.Builder
	b.WriteString("RedLab commands (virtual, deterministic, host-isolated):\n")
	for _, entry := range entries {
		fmt.Fprintf(&b, "%-18s [%s/%s] %s\n", entry.Name, entry.Pack, entry.Level, entry.Summary)
	}
	return Result{Stdout: b.String()}
}

func runMan(_ *Env, args []string, _ string) Result {
	if len(args) != 1 {
		return Result{ExitCode: 2, Stderr: "man: exactly one command is required\n"}
	}
	entry, ok := catalog.Lookup(args[0])
	if !ok {
		return Result{ExitCode: 1, Stderr: "man: no entry for " + args[0] + "\n"}
	}
	status := "recognized"
	if entry.Implemented {
		status = "emulated"
	}
	return Result{Stdout: fmt.Sprintf("NAME\n    %s - %s\n\nCOMPATIBILITY\n    pack: %s\n    level: %s\n    status: %s\n", entry.Name, entry.Summary, entry.Pack, entry.Level, status)}
}

func runDate(e *Env, args []string, _ string) Result {
	now := e.State.CurrentTime()
	if len(args) == 0 {
		return Result{Stdout: now.Format("Mon Jan 2 15:04:05 MST 2006\n")}
	}
	if args[0] == "+%s" {
		return Result{Stdout: strconv.FormatInt(now.Unix(), 10) + "\n"}
	}
	if strings.HasPrefix(args[0], "+") {
		format := strings.TrimPrefix(args[0], "+")
		format = strings.ReplaceAll(format, "%Y", "2006")
		format = strings.ReplaceAll(format, "%m", "01")
		format = strings.ReplaceAll(format, "%d", "02")
		format = strings.ReplaceAll(format, "%H", "15")
		format = strings.ReplaceAll(format, "%M", "04")
		format = strings.ReplaceAll(format, "%S", "05")
		format = strings.ReplaceAll(format, "%F", "2006-01-02")
		return Result{Stdout: now.Format(format) + "\n"}
	}
	return Result{ExitCode: 1, Stderr: "date: unsupported format\n"}
}

func runWhich(_ *Env, args []string, _ string) Result {
	if len(args) == 0 {
		return Result{ExitCode: 2, Stderr: "which: missing command\n"}
	}
	var out strings.Builder
	for _, name := range args {
		if _, ok := catalog.Lookup(name); !ok {
			return Result{ExitCode: 1, Stderr: name + ": not found\n"}
		}
		out.WriteString("/usr/bin/" + name + "\n")
	}
	return Result{Stdout: out.String()}
}

func runWC(e *Env, args []string, stdin string) Result {
	text := stdin
	name := "-"
	if len(args) > 0 {
		name = args[len(args)-1]
		if name != "-" {
			var err error
			text, err = e.State.ReadFile(name, e.User)
			if err != nil {
				return Result{ExitCode: 1, Stderr: "wc: " + err.Error() + "\n"}
			}
		}
	}
	lines := 0
	if text != "" {
		lines = strings.Count(text, "\n")
		if !strings.HasSuffix(text, "\n") {
			lines++
		}
	}
	words := len(strings.Fields(text))
	bytes := len([]byte(text))
	if len(args) == 0 || (len(args) == 1 && args[0] == "-") {
		return Result{Stdout: fmt.Sprintf("%d %d %d %s\n", lines, words, bytes, name)}
	}
	return Result{Stdout: fmt.Sprintf("%d %d %d %s\n", lines, words, bytes, name)}
}

func runSort(_ *Env, args []string, stdin string) Result {
	if len(args) > 0 {
		return Result{ExitCode: 1, Stderr: "sort: file operands are not supported; use redirection\n"}
	}
	if stdin == "" {
		return Result{}
	}
	lines := strings.Split(strings.TrimSuffix(stdin, "\n"), "\n")
	sort.Strings(lines)
	return Result{Stdout: strings.Join(lines, "\n") + "\n"}
}

func runUniq(_ *Env, args []string, stdin string) Result {
	if len(args) > 0 {
		return Result{ExitCode: 1, Stderr: "uniq: file operands are not supported; use redirection\n"}
	}
	if stdin == "" {
		return Result{}
	}
	lines := strings.Split(strings.TrimSuffix(stdin, "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(out) == 0 || out[len(out)-1] != line {
			out = append(out, line)
		}
	}
	return Result{Stdout: strings.Join(out, "\n") + "\n"}
}

func runCut(_ *Env, args []string, stdin string) Result {
	delimiter, field := "\t", 1
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-d" && i+1 < len(args):
			delimiter = args[i+1]
			i++
		case strings.HasPrefix(args[i], "-d"):
			delimiter = strings.TrimPrefix(args[i], "-d")
		case args[i] == "-f" && i+1 < len(args):
			field, _ = strconv.Atoi(args[i+1])
			i++
		case strings.HasPrefix(args[i], "-f"):
			field, _ = strconv.Atoi(strings.TrimPrefix(args[i], "-f"))
		}
	}
	if delimiter == "" || field < 1 {
		return Result{ExitCode: 2, Stderr: "cut: invalid delimiter or field\n"}
	}
	var out strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(stdin, "\n"), "\n") {
		parts := strings.Split(line, delimiter)
		if field <= len(parts) {
			out.WriteString(parts[field-1])
		}
		out.WriteByte('\n')
	}
	return Result{Stdout: out.String()}
}
func runTest(e *Env, a []string, _ string) Result {
	if len(a) == 0 {
		return Result{ExitCode: 2, Stderr: "test: missing argument\n"}
	}
	if len(a) == 2 && (a[0] == "-e" || a[0] == "-f" || a[0] == "-d") {
		file, err := e.State.Stat(a[1])
		ok := err == nil && ((a[0] == "-d" && file.Directory) || (a[0] != "-d" && !file.Directory))
		if ok {
			return Result{}
		}
		return Result{ExitCode: 1}
	}
	return Result{ExitCode: 2, Stderr: "test: unsupported expression\n"}
}
func runLS(e *Env, a []string, _ string) Result {
	target := "."
	long, all := false, false
	for _, v := range a {
		if strings.HasPrefix(v, "-") {
			if strings.Contains(v, "l") {
				long = true
			}
			if strings.Contains(v, "a") {
				all = true
			}
		} else {
			target = v
		}
	}
	files, err := e.State.List(target, e.User)
	if err != nil {
		return Result{ExitCode: 2, Stderr: "ls: " + err.Error() + "\n"}
	}
	var b strings.Builder
	for _, file := range files {
		if !all && strings.HasPrefix(pathBase(file.Path), ".") {
			continue
		}
		if long {
			fmt.Fprintf(&b, "%c%s %s %s %d %s\n", dirChar(file), formatMode(file.Mode, file.Directory), file.Owner, file.Group, len(file.Content), pathBase(file.Path))
		} else {
			b.WriteString(pathBase(file.Path) + "\n")
		}
	}
	return Result{Stdout: b.String()}
}
func runCat(e *Env, a []string, in string) Result {
	if len(a) == 0 {
		return Result{Stdout: in}
	}
	var b strings.Builder
	for _, name := range a {
		content, err := e.State.ReadFile(name, e.User)
		if err != nil {
			return Result{ExitCode: 1, Stderr: "cat: " + err.Error() + "\n"}
		}
		b.WriteString(content)
	}
	return Result{Stdout: b.String()}
}
func runHead(e *Env, a []string, in string) Result { return runLines(e, a, in, true) }
func runTail(e *Env, a []string, in string) Result { return runLines(e, a, in, false) }
func runLines(e *Env, a []string, in string, head bool) Result {
	count := 10
	var name string
	for _, v := range a {
		if strings.HasPrefix(v, "-") {
			n, _ := strconv.Atoi(strings.TrimLeft(v, "-n"))
			if n > 0 {
				count = n
			}
		} else {
			name = v
		}
	}
	if name != "" {
		var err error
		in, err = e.State.ReadFile(name, e.User)
		if err != nil {
			return Result{ExitCode: 1, Stderr: err.Error() + "\n"}
		}
	}
	lines := splitLines(in)
	if len(lines) == 0 {
		return Result{}
	}
	if count > len(lines) {
		count = len(lines)
	}
	if head {
		lines = lines[:count]
	} else {
		lines = lines[len(lines)-count:]
	}
	return Result{Stdout: strings.Join(lines, "")}
}
func runGrep(e *Env, a []string, in string) Result {
	if len(a) == 0 {
		return Result{ExitCode: 2, Stderr: "grep: missing pattern\n"}
	}
	pattern := a[0]
	names := a[1:]
	if len(names) > 0 {
		var b strings.Builder
		for _, name := range names {
			content, err := e.State.ReadFile(name, e.User)
			if err != nil {
				return Result{ExitCode: 2, Stderr: err.Error() + "\n"}
			}
			for _, line := range splitLines(content) {
				if strings.Contains(line, pattern) {
					b.WriteString(line)
				}
			}
		}
		if b.Len() == 0 {
			return Result{ExitCode: 1}
		}
		return Result{Stdout: b.String()}
	}
	var b strings.Builder
	for _, line := range splitLines(in) {
		if strings.Contains(line, pattern) {
			b.WriteString(line)
		}
	}
	if b.Len() == 0 {
		return Result{ExitCode: 1}
	}
	return Result{Stdout: b.String()}
}
func runTee(e *Env, a []string, in string) Result {
	if len(a) == 0 {
		return Result{Stdout: in}
	}
	for _, name := range a {
		if err := e.State.WriteFile(name, in, e.User, false); err != nil {
			return Result{ExitCode: 1, Stderr: "tee: " + err.Error() + "\n"}
		}
	}
	return Result{Stdout: in, Mutations: a}
}
func runStat(e *Env, a []string, _ string) Result {
	if len(a) == 0 {
		return Result{ExitCode: 1, Stderr: "stat: missing operand\n"}
	}
	f, err := e.State.Stat(a[len(a)-1])
	if err != nil {
		return Result{ExitCode: 1, Stderr: err.Error() + "\n"}
	}
	return Result{Stdout: fmt.Sprintf("  File: %s\n  Size: %d\n  Mode: (%04o)\n  Uid: %s\n  Gid: %s\n", f.Path, len(f.Content), f.Mode, f.Owner, f.Group)}
}
func runMkdir(e *Env, a []string, _ string) Result {
	if len(a) == 0 {
		return Result{ExitCode: 1, Stderr: "mkdir: missing operand\n"}
	}
	parents := false
	operands := make([]string, 0, len(a))
	for _, arg := range a {
		if arg == "-p" || arg == "--parents" {
			parents = true
			continue
		}
		operands = append(operands, arg)
	}
	if len(operands) == 0 {
		return Result{ExitCode: 1, Stderr: "mkdir: missing operand\n"}
	}
	for _, name := range operands {
		var err error
		if parents {
			err = e.State.MakeDirAll(name, e.User)
		} else {
			err = e.State.MakeDir(name, e.User)
		}
		if err != nil {
			return Result{ExitCode: 1, Stderr: "mkdir: " + err.Error() + "\n"}
		}
	}
	return Result{Mutations: append([]string(nil), operands...)}
}
func runTouch(e *Env, a []string, _ string) Result {
	if len(a) == 0 {
		return Result{ExitCode: 1, Stderr: "touch: missing file operand\n"}
	}
	for _, name := range a {
		if err := e.State.Touch(name, e.User); err != nil {
			return Result{ExitCode: 1, Stderr: "touch: " + err.Error() + "\n"}
		}
	}
	return Result{Mutations: append([]string(nil), a...)}
}
func runRM(e *Env, a []string, _ string) Result {
	if len(a) == 0 {
		return Result{ExitCode: 1, Stderr: "rm: missing operand\n"}
	}
	operands := make([]string, 0, len(a))
	recursive, force := false, false
	for _, name := range a {
		if name == "-f" || name == "--force" {
			force = true
			continue
		}
		if name == "-r" || name == "-R" || name == "--recursive" || name == "-rf" || name == "-fr" {
			recursive = true
			if strings.Contains(name, "f") {
				force = true
			}
			continue
		}
		operands = append(operands, name)
	}
	if len(operands) == 0 {
		return Result{ExitCode: 1, Stderr: "rm: missing operand\n"}
	}
	for _, name := range operands {
		file, statErr := e.State.Stat(name)
		if statErr != nil && force {
			continue
		}
		if statErr == nil && file.Directory && !recursive {
			return Result{ExitCode: 1, Stderr: "rm: cannot remove '" + name + "': Is a directory\n"}
		}
		if err := e.State.Remove(name, e.User); err != nil {
			if force && strings.Contains(err.Error(), "No such file") {
				continue
			}
			return Result{ExitCode: 1, Stderr: "rm: " + err.Error() + "\n"}
		}
	}
	return Result{Mutations: append([]string(nil), operands...)}
}
func runChmod(e *Env, a []string, _ string) Result {
	if len(a) < 2 {
		return Result{ExitCode: 1, Stderr: "chmod: missing operand\n"}
	}
	for _, name := range a[1:] {
		if err := e.State.Chmod(name, a[0], e.User); err != nil {
			return Result{ExitCode: 1, Stderr: "chmod: " + err.Error() + "\n"}
		}
	}
	return Result{Mutations: a[1:]}
}
func runChown(e *Env, a []string, _ string) Result {
	if len(a) < 2 {
		return Result{ExitCode: 1, Stderr: "chown: missing operand\n"}
	}
	parts := strings.SplitN(a[0], ":", 2)
	for _, name := range a[1:] {
		group := ""
		if len(parts) > 1 {
			group = parts[1]
		}
		if err := e.State.Chown(name, parts[0], group, e.User); err != nil {
			return Result{ExitCode: 1, Stderr: "chown: " + err.Error() + "\n"}
		}
	}
	return Result{Mutations: a[1:]}
}

func runCopy(e *Env, a []string, _ string) Result {
	recursive := false
	operands := make([]string, 0, len(a))
	for _, arg := range a {
		if arg == "-r" || arg == "-R" || arg == "--recursive" {
			recursive = true
			continue
		}
		operands = append(operands, arg)
	}
	if len(operands) != 2 {
		return Result{ExitCode: 2, Stderr: "cp: usage: cp [-r] SOURCE DEST\n"}
	}
	if err := e.State.CopyRecursive(operands[0], operands[1], e.User, false, recursive); err != nil {
		return Result{ExitCode: 1, Stderr: "cp: " + err.Error() + "\n"}
	}
	return Result{Mutations: []string{operands[0] + " -> " + operands[1]}}
}

func runMove(e *Env, a []string, _ string) Result {
	operands := make([]string, 0, len(a))
	for _, arg := range a {
		if arg == "-f" || arg == "--force" {
			continue
		}
		operands = append(operands, arg)
	}
	if len(operands) != 2 {
		return Result{ExitCode: 2, Stderr: "mv: usage: mv SOURCE DEST\n"}
	}
	if err := e.State.CopyRecursive(operands[0], operands[1], e.User, true, true); err != nil {
		return Result{ExitCode: 1, Stderr: "mv: " + err.Error() + "\n"}
	}
	return Result{Mutations: []string{operands[0] + " -> " + operands[1]}}
}

func runRmdir(e *Env, a []string, _ string) Result {
	if len(a) != 1 {
		return Result{ExitCode: 2, Stderr: "rmdir: usage: rmdir DIRECTORY\n"}
	}
	file, err := e.State.Stat(a[0])
	if err != nil || !file.Directory {
		return Result{ExitCode: 1, Stderr: "rmdir: directory not found\n"}
	}
	children, err := e.State.List(a[0], e.User)
	if err != nil {
		return Result{ExitCode: 1, Stderr: "rmdir: " + err.Error() + "\n"}
	}
	if len(children) > 0 {
		return Result{ExitCode: 1, Stderr: "rmdir: Directory not empty\n"}
	}
	if err := e.State.Remove(a[0], e.User); err != nil {
		return Result{ExitCode: 1, Stderr: "rmdir: " + err.Error() + "\n"}
	}
	return Result{Mutations: a}
}

func runFind(e *Env, a []string, _ string) Result {
	root := "."
	name, kind := "", ""
	for i := 0; i < len(a); i++ {
		switch a[i] {
		case "-name":
			if i+1 >= len(a) {
				return Result{ExitCode: 2, Stderr: "find: -name needs a pattern\n"}
			}
			name = a[i+1]
			i++
		case "-type":
			if i+1 >= len(a) {
				return Result{ExitCode: 2, Stderr: "find: -type needs a value\n"}
			}
			kind = a[i+1]
			i++
		default:
			if !strings.HasPrefix(a[i], "-") {
				root = a[i]
			}
		}
	}
	files, err := e.State.Walk(root, e.User)
	if err != nil {
		return Result{ExitCode: 1, Stderr: "find: " + err.Error() + "\n"}
	}
	var out strings.Builder
	for _, file := range files {
		if kind == "f" && file.Directory || kind == "d" && !file.Directory {
			continue
		}
		if name != "" {
			matched, _ := path.Match(name, path.Base(file.Path))
			if !matched {
				continue
			}
		}
		out.WriteString(file.Path + "\n")
	}
	return Result{Stdout: out.String()}
}

func runBaseName(_ *Env, args []string, _ string) Result {
	if len(args) != 1 {
		return Result{ExitCode: 1, Stderr: "basename: missing operand\n"}
	}
	return Result{Stdout: path.Base(args[0]) + "\n"}
}

func runDirName(_ *Env, args []string, _ string) Result {
	if len(args) != 1 {
		return Result{ExitCode: 1, Stderr: "dirname: missing operand\n"}
	}
	return Result{Stdout: path.Dir(args[0]) + "\n"}
}

func runRealPath(e *Env, args []string, _ string) Result {
	if len(args) != 1 {
		return Result{ExitCode: 1, Stderr: "realpath: missing operand\n"}
	}
	resolved, err := system.NormalizePath(args[0], e.CWD)
	if err != nil {
		return Result{ExitCode: 1, Stderr: "realpath: " + err.Error() + "\n"}
	}
	if _, err := e.State.Stat(resolved); err != nil {
		return Result{ExitCode: 1, Stderr: "realpath: " + err.Error() + "\n"}
	}
	return Result{Stdout: resolved + "\n"}
}

func runTree(e *Env, args []string, _ string) Result {
	root := "."
	if len(args) > 0 {
		root = args[len(args)-1]
	}
	resolved, err := system.NormalizePath(root, e.CWD)
	if err != nil {
		return Result{ExitCode: 1, Stderr: "tree: " + err.Error() + "\n"}
	}
	files, err := e.State.Walk(root, e.User)
	if err != nil {
		return Result{ExitCode: 1, Stderr: "tree: " + err.Error() + "\n"}
	}
	var b strings.Builder
	b.WriteString(resolved + "\n")
	directories, regular := 0, 0
	for _, file := range files {
		if file.Path == resolved {
			continue
		}
		relative := strings.TrimPrefix(file.Path, strings.TrimSuffix(resolved, "/")+"/")
		depth := strings.Count(relative, "/")
		b.WriteString(strings.Repeat("    ", depth) + "|-- " + path.Base(file.Path) + "\n")
		if file.Directory {
			directories++
		} else {
			regular++
		}
	}
	fmt.Fprintf(&b, "\n%d directories, %d files\n", directories, regular)
	return Result{Stdout: b.String()}
}
func runID(e *Env, _ []string, _ string) Result {
	u, ok := e.State.Users[e.User]
	if !ok {
		return Result{ExitCode: 1, Stderr: "id: no such user\n"}
	}
	return Result{Stdout: fmt.Sprintf("uid=%d(%s) gid=%d(%s) groups=%s\n", u.UID, u.Name, u.GID, u.Name, strings.Join(u.Groups, ","))}
}
func runGroups(e *Env, _ []string, _ string) Result {
	u, ok := e.State.Users[e.User]
	if !ok {
		return Result{ExitCode: 1}
	}
	return Result{Stdout: e.User + " : " + strings.Join(u.Groups, " ") + "\n"}
}

func runUsermod(e *Env, a []string, _ string) Result {
	if len(a) < 3 || (a[0] != "-aG" && a[0] != "-G") {
		return Result{ExitCode: 2, Stderr: "usermod: usage: usermod -aG GROUP USER\n"}
	}
	groups := strings.Split(a[1], ",")
	if len(groups) == 0 || groups[0] == "" {
		return Result{ExitCode: 2, Stderr: "usermod: group is required\n"}
	}
	for _, group := range groups {
		if err := e.State.AddUserToGroup(a[2], group, e.User); err != nil {
			return Result{ExitCode: 1, Stderr: err.Error() + "\n"}
		}
	}
	return Result{Mutations: []string{"usergroups:" + a[2]}}
}

func runUseradd(e *Env, a []string, _ string) Result {
	if len(a) != 1 || strings.HasPrefix(a[0], "-") {
		return Result{ExitCode: 2, Stderr: "useradd: usage: useradd USER\n"}
	}
	if err := e.State.AddUser(a[0], e.User); err != nil {
		return Result{ExitCode: 1, Stderr: "useradd: " + err.Error() + "\n"}
	}
	return Result{Mutations: []string{"useradd:" + a[0]}}
}

func runSudo(e *Env, a []string, in string) Result {
	if len(a) == 0 {
		return Result{ExitCode: 1, Stderr: "sudo: a command is required\n"}
	}
	if e.User != "root" && !e.State.SudoAllowed(e.User, a[0]) {
		return Result{ExitCode: 1, Stderr: fmt.Sprintf("%s is not in the sudoers file. This incident will be reported.\n", e.User)}
	}
	old := e.User
	e.User = "root"
	result := executeNested(e, a, in)
	e.User = old
	return result
}
func executeNested(e *Env, a []string, in string) Result {
	if len(a) == 0 {
		return Result{}
	}
	name := a[0]
	if name == "sudo" {
		return Result{ExitCode: 1, Stderr: "sudo: nested sudo is not supported\n"}
	}
	return defaultRegistry.Run(name, e, a[1:], in)
}

var defaultRegistry = NewRegistry()

func init() { RegisterCore(defaultRegistry) }
func runSystemctl(e *Env, a []string, _ string) Result {
	if len(a) < 1 {
		return Result{ExitCode: 1, Stderr: "systemctl: missing command\n"}
	}
	verb := a[0]
	names := a[1:]
	if len(names) == 0 && verb != "daemon-reload" {
		return Result{ExitCode: 1, Stderr: "systemctl: missing unit\n"}
	}
	var b strings.Builder
	for _, name := range names {
		if !strings.Contains(name, ".") {
			name += ".service"
		}
		switch verb {
		case "status":
			service, ok := e.State.Service(name)
			if !ok {
				return Result{ExitCode: 4, Stderr: "Unit " + name + " could not be found.\n"}
			}
			fmt.Fprintf(&b, "● %s - %s\n   Loaded: loaded; %s; vendor preset: disabled\n   Active: %s\n", name, name, enabledText(service.Enabled), service.State)
		case "start":
			if r := e.State.StartService(name, e.User); r != nil {
				return Result{ExitCode: 1, Stderr: r.Error() + "\n"}
			}
		case "restart":
			if r := e.State.StopService(name, e.User); r != nil {
				return Result{ExitCode: 1, Stderr: r.Error() + "\n"}
			}
			if r := e.State.StartService(name, e.User); r != nil {
				return Result{ExitCode: 1, Stderr: r.Error() + "\n"}
			}
		case "stop":
			if r := e.State.StopService(name, e.User); r != nil {
				return Result{ExitCode: 1, Stderr: r.Error() + "\n"}
			}
		case "enable":
			if r := e.State.EnableService(name, e.User, true); r != nil {
				return Result{ExitCode: 1, Stderr: r.Error() + "\n"}
			}
		case "disable":
			if r := e.State.EnableService(name, e.User, false); r != nil {
				return Result{ExitCode: 1, Stderr: r.Error() + "\n"}
			}
		default:
			return Result{ExitCode: 1, Stderr: "systemctl: unsupported operation " + verb + "\n"}
		}
	}
	return Result{Stdout: b.String(), Mutations: names}
}
func runJournal(e *Env, a []string, _ string) Result {
	unit := ""
	for _, v := range a {
		if strings.HasPrefix(v, "-u") {
			unit = strings.TrimPrefix(v, "-u")
		} else if strings.HasPrefix(v, "httpd") {
			unit = v
		}
	}
	var b strings.Builder
	for _, j := range e.State.JournalEntries(unit) {
		fmt.Fprintf(&b, "%s %s %s[%s]: %s\n", j.Timestamp.Format("Jan 02 15:04:05"), e.State.Hostname, j.Unit, j.Priority, j.Message)
	}
	return Result{Stdout: b.String()}
}
func runLogger(e *Env, a []string, _ string) Result {
	e.State.AddJournal("", "info", strings.Join(a, " "), e.User)
	return Result{}
}

func runHostname(e *Env, _ []string, _ string) Result {
	return Result{Stdout: e.State.Hostname + "\n"}
}

func runHost(e *Env, a []string, _ string) Result {
	if len(a) != 1 {
		return Result{ExitCode: 2, Stderr: "host: usage: host NAME\n"}
	}
	address, ok := e.State.Resolve(a[0])
	if !ok {
		return Result{ExitCode: 1, Stderr: "Host " + a[0] + " not found\n"}
	}
	return Result{Stdout: fmt.Sprintf("%s has address %s\n", a[0], address)}
}

func runRPM(e *Env, a []string, _ string) Result {
	if len(a) == 1 && a[0] == "-qa" {
		var out strings.Builder
		for _, pkg := range e.State.PackagesSnapshot() {
			fmt.Fprintf(&out, "%s-%s\n", pkg.Name, pkg.Version)
		}
		return Result{Stdout: out.String()}
	}
	if len(a) == 2 && a[0] == "-q" {
		for _, pkg := range e.State.PackagesSnapshot() {
			if pkg.Name == a[1] || pkg.Name+"-"+pkg.Version == a[1] {
				return Result{Stdout: pkg.Name + "-" + pkg.Version + "\n"}
			}
		}
		return Result{ExitCode: 1, Stderr: "package " + a[1] + " is not installed\n"}
	}
	return Result{ExitCode: 2, Stderr: "rpm: supported forms are rpm -q NAME and rpm -qa\n"}
}

func runDNF(e *Env, a []string, _ string) Result {
	if len(a) == 2 && a[0] == "list" && a[1] == "installed" {
		var out strings.Builder
		out.WriteString("Installed Packages\n")
		for _, pkg := range e.State.PackagesSnapshot() {
			fmt.Fprintf(&out, "%s.x86_64 %s @virtual\n", pkg.Name, pkg.Version)
		}
		return Result{Stdout: out.String()}
	}
	return Result{ExitCode: 2, Stderr: "dnf: supported form is dnf list installed\n"}
}

func runPS(e *Env, args []string, _ string) Result {
	processes := e.State.ProcessesSnapshot()
	wide := false
	full := false
	for _, arg := range args {
		switch arg {
		case "aux":
			wide = true
		case "-ef":
			full = true
		case "-e", "-f":
			full = true
		case "-a", "-x":
			wide = true
		default:
			if strings.HasPrefix(arg, "-") {
				return Result{ExitCode: 1, Stderr: "ps: unsupported option " + arg + "\n"}
			}
		}
	}
	var out strings.Builder
	if wide {
		out.WriteString("USER       PID %CPU %MEM STAT COMMAND\n")
	} else if full {
		out.WriteString("UID          PID    PPID  C STIME TTY          TIME CMD\n")
	} else {
		out.WriteString("PID TTY          TIME CMD\n")
	}
	for _, process := range processes {
		if wide {
			fmt.Fprintf(&out, "%-10s %4d %4.1f %4.1f %-4s %s\n", process.User, process.PID, float64(process.CPUSeconds), float64(process.MemoryBytes)/1048576, process.State, process.Command)
		} else if full {
			fmt.Fprintf(&out, "%-10s %6d %7d  0 00:00 ?        00:00:00 %s\n", process.User, process.PID, process.PPID, process.Command)
		} else {
			fmt.Fprintf(&out, "%3d ?        00:00:%02d %s\n", process.PID, process.CPUSeconds, process.Command)
		}
	}
	return Result{Stdout: out.String()}
}

func runPSTree(e *Env, _ []string, _ string) Result {
	var out strings.Builder
	processes := e.State.ProcessesSnapshot()
	if len(processes) == 0 {
		return Result{}
	}
	out.WriteString("systemd(1)")
	for _, process := range processes[1:] {
		name := path.Base(process.Command)
		name = strings.TrimSuffix(name, ".service")
		fmt.Fprintf(&out, "---%s(%d)", name, process.PID)
	}
	out.WriteByte('\n')
	return Result{Stdout: out.String()}
}

func runPGrep(e *Env, args []string, _ string) Result {
	all := false
	pattern := ""
	for _, arg := range args {
		if arg == "-a" {
			all = true
			continue
		}
		if pattern == "" {
			pattern = arg
		} else {
			return Result{ExitCode: 2, Stderr: "pgrep: too many patterns\n"}
		}
	}
	if pattern == "" {
		return Result{ExitCode: 2, Stderr: "pgrep: pattern is required\n"}
	}
	var out strings.Builder
	found := 0
	for _, process := range e.State.ProcessesSnapshot() {
		if strings.Contains(process.Command, pattern) || (strings.TrimSuffix(path.Base(process.Command), ".service") == pattern) || (process.PID == 1 && pattern == "systemd") {
			if all {
				fmt.Fprintf(&out, "%d %s\n", process.PID, process.Command)
			} else {
				fmt.Fprintf(&out, "%d\n", process.PID)
			}
			found++
		}
	}
	result := Result{Stdout: out.String()}
	if found == 0 {
		result.ExitCode = 1
	}
	return result
}

func runKill(e *Env, args []string, _ string) Result {
	signal := "TERM"
	operands := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			signal = strings.TrimPrefix(arg, "-")
			continue
		}
		operands = append(operands, arg)
	}
	if len(operands) == 0 {
		return Result{ExitCode: 2, Stderr: "kill: usage: kill [-SIGNAL] PID\n"}
	}
	mutations := make([]string, 0, len(operands))
	for _, operand := range operands {
		pid, err := strconv.Atoi(operand)
		if err != nil {
			return Result{ExitCode: 2, Stderr: "kill: invalid pid " + operand + "\n"}
		}
		if err := e.State.SignalProcess(pid, signal, e.User); err != nil {
			return Result{ExitCode: 1, Stderr: "kill: " + err.Error() + "\n"}
		}
		mutations = append(mutations, operand)
	}
	return Result{Mutations: mutations}
}

func runPKill(e *Env, args []string, _ string) Result { return runKillAll(e, args, "") }

func runKillAll(e *Env, args []string, _ string) Result {
	if len(args) != 1 {
		return Result{ExitCode: 2, Stderr: "killall: usage: killall NAME\n"}
	}
	pattern := args[0]
	matched := 0
	for _, process := range e.State.ProcessesSnapshot() {
		name := strings.TrimSuffix(path.Base(process.Command), ".service")
		if process.Command == pattern || name == pattern || strings.TrimSuffix(name, ".service") == strings.TrimSuffix(pattern, ".service") {
			if err := e.State.SignalProcess(process.PID, "TERM", e.User); err != nil {
				return Result{ExitCode: 1, Stderr: "killall: " + err.Error() + "\n"}
			}
			matched++
		}
	}
	if matched == 0 {
		return Result{ExitCode: 1, Stderr: "killall: " + pattern + ": no process found\n"}
	}
	return Result{Mutations: []string{pattern}}
}

func runTop(e *Env, _ []string, _ string) Result {
	return Result{Stdout: "top - virtual RedLab process view\n" + runPS(e, []string{"aux"}, "").Stdout}
}

func runDF(e *Env, _ []string, _ string) Result {
	usedBytes, _ := e.State.VirtualUsage()
	const totalBytes = 1024 * 1024
	usedBlocks := (usedBytes + 1023) / 1024
	available := totalBytes/1024 - usedBlocks
	if available < 0 {
		available = 0
	}
	return Result{Stdout: fmt.Sprintf("Filesystem     1024-blocks  Used Available Capacity Mounted on\nvirtual              %d %d %d %d%% /\n", totalBytes/1024, usedBlocks, available, usedBytes*100/totalBytes)}
}

func runDU(e *Env, args []string, _ string) Result {
	target := "."
	human := false
	for _, arg := range args {
		if arg == "-h" || arg == "-sh" || arg == "-hs" {
			human = true
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			target = arg
		}
	}
	files, err := e.State.Walk(target, e.User)
	if err != nil {
		return Result{ExitCode: 1, Stderr: "du: " + err.Error() + "\n"}
	}
	bytes := 0
	for _, file := range files {
		bytes += len(file.Content)
	}
	blocks := (bytes + 1023) / 1024
	value := strconv.Itoa(blocks)
	if human {
		value = fmt.Sprintf("%dK", blocks)
	}
	return Result{Stdout: value + "\t" + target + "\n"}
}

func runFree(_ *Env, args []string, _ string) Result {
	unit := "Ki"
	if len(args) > 0 && (args[0] == "-m" || args[0] == "--mebi") {
		unit = "Mi"
	}
	if unit == "Mi" {
		return Result{Stdout: "               total        used        free      shared  buff/cache   available\nMem:            2048         320        1536          16         192        1728\nSwap:           1024           0        1024\n"}
	}
	return Result{Stdout: "               total        used        free      shared  buff/cache   available\nMem:         2097152      327680     1572864       16384      196608     1769472\nSwap:        1048576           0     1048576\n"}
}

func runUptime(e *Env, _ []string, _ string) Result {
	return Result{Stdout: fmt.Sprintf(" %s up 1 day,  0:00,  1 user,  load average: 0.00, 0.01, 0.05\n", e.State.CurrentTime().Format("15:04:05"))}
}

func runUname(e *Env, args []string, _ string) Result {
	architecture := e.State.Architecture
	if architecture == "" {
		architecture = "x86_64"
	}
	if len(args) == 0 {
		return Result{Stdout: "Linux\n"}
	}
	switch args[0] {
	case "-r", "--kernel-release":
		return Result{Stdout: "4.18.0-553.el8_10." + architecture + "\n"}
	case "-m", "--machine":
		return Result{Stdout: architecture + "\n"}
	case "-n", "--nodename":
		return Result{Stdout: e.State.Hostname + "\n"}
	case "-a", "--all":
		return Result{Stdout: fmt.Sprintf("Linux %s 4.18.0-553.el8_10.%s #1 SMP %s %s %s GNU/Linux\n", e.State.Hostname, architecture, architecture, architecture, architecture)}
	default:
		return Result{ExitCode: 1, Stderr: "uname: unsupported option '" + args[0] + "'\n"}
	}
}

func runIP(e *Env, a []string, _ string) Result {
	if len(a) > 0 && a[0] != "addr" && a[0] != "address" {
		return Result{ExitCode: 1, Stderr: "ip: unsupported object\n"}
	}
	var b strings.Builder
	for i, iface := range e.State.Network.Interfaces {
		fmt.Fprintf(&b, "%d: %s: <%s> state %s\n", i+1, iface.Name, strings.ToUpper(iface.State), strings.ToUpper(iface.State))
		for _, addr := range iface.Addresses {
			fmt.Fprintf(&b, "    inet %s\n", addr)
		}
	}
	return Result{Stdout: b.String()}
}
func runSS(e *Env, _ []string, _ string) Result {
	var b strings.Builder
	keys := make([]string, 0, len(e.State.Network.OpenPorts))
	for key := range e.State.Network.OpenPorts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if e.State.Network.OpenPorts[key] {
			parts := strings.Split(key, ":")
			if len(parts) > 1 {
				fmt.Fprintf(&b, "LISTEN 0 128 *:%s\n", parts[len(parts)-1])
			}
		}
	}
	return Result{Stdout: b.String()}
}
func runPing(e *Env, a []string, _ string) Result {
	if len(a) == 0 {
		return Result{ExitCode: 2, Stderr: "ping: missing host\n"}
	}
	host := a[len(a)-1]
	if _, ok := e.State.Resolve(host); !ok {
		return Result{ExitCode: 1, Stderr: "ping: unknown host " + host + "\n"}
	}
	return Result{Stdout: fmt.Sprintf("PING %s (%s): virtual response\n--- %s ping statistics ---\n1 packets transmitted, 1 received, 0%% packet loss\n", host, lookup(e, host), host)}
}
func runDig(e *Env, a []string, _ string) Result {
	if len(a) == 0 {
		return Result{ExitCode: 1, Stderr: "dig: missing name\n"}
	}
	host := a[len(a)-1]
	address, ok := e.State.Resolve(host)
	if !ok {
		return Result{ExitCode: 1, Stderr: ";; NXDOMAIN\n"}
	}
	return Result{Stdout: fmt.Sprintf(";; ANSWER SECTION:\n%s. 60 IN A %s\n", host, address)}
}
func runCurl(e *Env, a []string, _ string) Result {
	if len(a) == 0 {
		return Result{ExitCode: 2, Stderr: "curl: no URL specified\n"}
	}
	raw := a[len(a)-1]
	response, err := e.State.HTTPGet(raw)
	if err != nil {
		return Result{ExitCode: 6, Stderr: err.Error() + "\n"}
	}
	if response.Status >= 400 {
		return Result{ExitCode: 22, Stdout: response.Body, Stderr: fmt.Sprintf("curl: (22) HTTP error %d\n", response.Status)}
	}
	return Result{Stdout: response.Body}
}
func runFirewall(e *Env, a []string, _ string) Result {
	if len(a) == 0 {
		return Result{ExitCode: 1, Stderr: "firewall-cmd: missing option\n"}
	}
	zone := e.State.Network.DefaultZone
	service := ""
	permanent := false
	for _, v := range a {
		if strings.HasPrefix(v, "--zone=") {
			zone = strings.TrimPrefix(v, "--zone=")
		}
		if strings.HasPrefix(v, "--add-service=") {
			service = strings.TrimPrefix(v, "--add-service=")
		}
		if v == "--permanent" {
			permanent = true
		}
		if strings.HasPrefix(v, "--query-service=") {
			service = strings.TrimPrefix(v, "--query-service=")
			if e.State.FirewallAllowed(zone, service) {
				return Result{Stdout: "yes\n"}
			}
			return Result{ExitCode: 1, Stdout: "no\n"}
		}
	}
	if service != "" {
		if err := e.State.AllowFirewall(zone, service, e.User, permanent); err != nil {
			return Result{ExitCode: 1, Stderr: err.Error() + "\n"}
		}
		return Result{Stdout: "success\n", Mutations: []string{zone + ":" + service}}
	}
	return Result{ExitCode: 1, Stderr: "firewall-cmd: unsupported option\n"}
}
func runSetEnforce(e *Env, a []string, _ string) Result {
	if len(a) != 1 {
		return Result{ExitCode: 1, Stderr: "setenforce: usage: setenforce [ Enforcing | Permissive | 1 | 0 ]\n"}
	}
	mode := strings.ToLower(a[0])
	if mode == "1" {
		mode = "enforcing"
	}
	if mode == "0" {
		mode = "permissive"
	}
	if err := e.State.SetSELinux(mode, e.User); err != nil {
		return Result{ExitCode: 1, Stderr: err.Error() + "\n"}
	}
	return Result{Mutations: []string{"selinux:" + mode}}
}
func runSEStatus(e *Env, _ []string, _ string) Result {
	return Result{Stdout: fmt.Sprintf("SELinux status:                 enabled\nSELinuxfs mount:                /sys/fs/selinux\nSELinux mode:                   %s\nCurrent policy name:            targeted\n", e.State.SELinuxMode())}
}
func runRestorecon(e *Env, a []string, _ string) Result {
	for _, name := range a {
		f, err := e.State.Stat(name)
		if err != nil {
			return Result{ExitCode: 1, Stderr: err.Error() + "\n"}
		}
		_ = f
	}
	return Result{Stdout: ""}
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
func pathBase(name string) string {
	if name == "/" {
		return "/"
	}
	return name[strings.LastIndex(name, "/")+1:]
}
func dirChar(f system.File) rune {
	if f.Directory {
		return 'd'
	}
	return '-'
}
func formatMode(mode uint32, dir bool) string {
	prefix := "-"
	if dir {
		prefix = "d"
	}
	return prefix + fmt.Sprintf("%03o", mode)
}
func enabledText(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
func lookup(e *Env, host string) string { address, _ := e.State.Resolve(host); return address }
