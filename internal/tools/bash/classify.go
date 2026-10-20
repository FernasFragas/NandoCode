package bash

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type classification struct {
	readOnly    bool
	destructive bool
}

func classify(command string) classification {
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return classification{readOnly: false, destructive: true}
	}

	seenCommand := false
	readOnly := true
	destructive := false
	syntax.Walk(file, func(node syntax.Node) bool {
		if !readOnly && destructive {
			return false
		}
		switch n := node.(type) {
		case *syntax.Stmt:
			if len(n.Redirs) > 0 {
				readOnly = false
				destructive = true
				return false
			}
		case *syntax.CallExpr:
			name, ok := commandName(n)
			if !ok {
				readOnly = false
				destructive = true
				return false
			}
			seenCommand = true
			args, allLiteral := commandArgs(n)
			// VAR=x cmd can change what cmd runs (PATH, LD_PRELOAD, GIT_PAGER, ...).
			unsafe := len(n.Assigns) > 0 || !allLiteral
			name, args, resolved, tainted := unwrapWrappers(name, args)
			unsafe = unsafe || tainted
			if !resolved {
				// A wrapper whose inner command we cannot read: assume the worst.
				readOnly = false
				destructive = true
				return false
			}
			if isDestructiveCommand(name, args) {
				readOnly = false
				destructive = true
				return false
			}
			if isNeutralCommand(name) {
				return true
			}
			if unsafe {
				readOnly = false
				return true
			}
			if !isReadOnlyCommand(name, args) {
				readOnly = false
			}
		case *syntax.CmdSubst, *syntax.ProcSubst:
			readOnly = false
			destructive = true
			return false
		}
		return true
	})
	if !seenCommand {
		return classification{readOnly: false, destructive: true}
	}
	return classification{readOnly: readOnly, destructive: destructive}
}

func commandName(call *syntax.CallExpr) (string, bool) {
	if len(call.Args) == 0 {
		return "", false
	}
	return literalWord(call.Args[0])
}

// commandArgs returns the call's arguments with quoting removed. allLiteral is
// false when any argument depends on expansion (variables, substitutions, braces,
// escapes, ANSI-C quotes); such arguments appear as "" and the command cannot be
// proven read-only.
func commandArgs(call *syntax.CallExpr) (args []string, allLiteral bool) {
	allLiteral = true
	args = make([]string, 0, len(call.Args)-1)
	for _, word := range call.Args[1:] {
		if lit, ok := literalWord(word); ok {
			args = append(args, lit)
		} else {
			args = append(args, "")
			allLiteral = false
		}
	}
	return args, allLiteral
}

// literalWord returns the value of a word made only of literal text, resolving
// single and double quotes. Anything that needs the shell to expand it is not literal.
func literalWord(word *syntax.Word) (string, bool) {
	var b strings.Builder
	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if strings.ContainsAny(p.Value, "\\{") {
				return "", false
			}
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			if p.Dollar {
				return "", false
			}
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			if p.Dollar {
				return "", false
			}
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok || strings.ContainsAny(lit.Value, "\\") {
					return "", false
				}
				b.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	return b.String(), true
}

func isNeutralCommand(name string) bool {
	switch name {
	case "echo", "printf", "true", "false", "pwd":
		return true
	default:
		return false
	}
}

func isReadOnlyCommand(name string, args []string) bool {
	switch name {
	case "ls", "cat", "head", "tail", "wc", "cut", "grep", "egrep", "fgrep", "stat", "file", "du", "df", "printenv", "which", "type", "uname":
		return true
	case "env":
		// Only reached when env has no inner command (see unwrapWrappers).
		return true
	case "rg":
		return !hasAnyFlag(args, "--pre", "--pre-glob")
	case "find":
		return !hasAnyFlag(args, findWriteFlags...)
	case "sort":
		return !sortWrites(args)
	case "uniq":
		return positionalCount(args) <= 1
	case "date":
		return !dateSetsClock(args)
	case "git":
		return len(args) > 0 && gitReadOnly(args)
	case "go":
		return len(args) > 0 && goReadOnly(args)
	default:
		return false
	}
}

var findWriteFlags = []string{"-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0", "-fprintf", "-fls"}

// hasAnyFlag reports whether args contains one of flags. Long options ("--x")
// also match any abbreviation of three or more characters, because getopt-style
// parsers accept "--out" for "--output"; the "=value" part is ignored. Single-dash
// flags (find's -delete, go's -w) must match exactly.
func hasAnyFlag(args []string, flags ...string) bool {
	for _, a := range args {
		opt, _, _ := strings.Cut(a, "=")
		for _, f := range flags {
			if opt == f {
				return true
			}
			if strings.HasPrefix(f, "--") && strings.HasPrefix(opt, "--") && len(opt) > 2 && strings.HasPrefix(f, opt) {
				return true
			}
		}
	}
	return false
}

func positionalCount(args []string) int {
	n := 0
	for _, a := range args {
		if a == "" || !strings.HasPrefix(a, "-") {
			n++
		}
	}
	return n
}

// sortWrites reports whether sort would write a file (-o/--output) or run a
// helper program (--compress-program). Short-option clusters containing "o" are
// treated as writing, which errs on the side of asking.
func sortWrites(args []string) bool {
	if hasAnyFlag(args, "--output", "--compress-program") {
		return true
	}
	for _, a := range args {
		if len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.Contains(a, "o") {
			return true
		}
	}
	return false
}

func dateSetsClock(args []string) bool {
	if hasAnyFlag(args, "--set") {
		return true
	}
	for _, a := range args {
		if len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.Contains(a, "s") {
			return true
		}
	}
	return false
}

// gitReadOnly decides from the subcommand and its arguments. args[0] is the
// subcommand; git's global options before it make the call unknown, hence not
// read-only.
func gitReadOnly(args []string) bool {
	sub, rest := args[0], args[1:]
	if hasAnyFlag(rest, "--output", "--open-files-in-pager", "--ext-diff", "--textconv") {
		return false
	}
	if sub == "grep" {
		// -O[PAGER] runs a pager command; it may sit in a short-option cluster.
		for _, a := range rest {
			if len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.Contains(a, "O") {
				return false
			}
		}
	}
	switch sub {
	case "status", "diff", "log", "show", "rev-parse", "ls-files", "grep":
		return true
	case "branch":
		return gitBranchReadOnly(rest)
	case "remote":
		return gitRemoteReadOnly(rest)
	default:
		return false
	}
}

func gitBranchReadOnly(args []string) bool {
	listing := false
	for _, a := range args {
		switch a {
		case "--list", "-l", "--contains", "--no-contains", "--merged", "--no-merged", "--points-at":
			listing = true
		}
	}
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			if !listing {
				return false // "git branch NAME" creates a branch
			}
			continue
		}
		flag, _, _ := strings.Cut(a, "=")
		switch flag {
		case "-a", "-r", "-v", "-vv", "-av", "-ar", "-rv", "--all", "--remotes", "--verbose",
			"--list", "-l", "--show-current", "--merged", "--no-merged", "--contains", "--no-contains",
			"--points-at", "--sort", "--format", "--color", "--no-color", "--column", "--no-column",
			"-i", "--ignore-case", "--abbrev", "--no-abbrev":
		default:
			return false
		}
	}
	return true
}

func gitRemoteReadOnly(args []string) bool {
	if len(args) == 0 {
		return true
	}
	return len(args) == 1 && (args[0] == "-v" || args[0] == "--verbose")
}

func goReadOnly(args []string) bool {
	switch args[0] {
	case "test":
		return !goTestRunsOrWrites(args[1:])
	case "list", "version":
		return true
	case "env":
		return !hasAnyFlag(args[1:], "-w", "-u")
	default:
		return false
	}
}

// goTestRunsOrWrites reports go test flags that run another program (-exec,
// -toolexec) or write files (-o, -c, -*profile, -trace).
func goTestRunsOrWrites(args []string) bool {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimLeft(a, "-"), "=")
		switch name {
		case "exec", "toolexec", "o", "c", "trace":
			return true
		}
		if strings.Contains(name, "profile") {
			return true
		}
	}
	return false
}

func isDestructiveCommand(name string, args []string) bool {
	switch name {
	case "rm", "rmdir", "mv", "cp", "mkdir", "touch", "chmod", "chown", "dd", "tee", "truncate", "curl", "wget", "ssh", "scp", "sudo":
		return true
	case "find":
		return hasAnyFlag(args, findWriteFlags...)
	case "git":
		return len(args) > 0 && !gitReadOnly(args)
	case "go":
		return len(args) > 0 && !goReadOnly(args)
	default:
		return false
	}
}

// unwrapWrappers strips commands that only run another command (env, nice,
// timeout, nohup, xargs, command, builtin, exec) so the inner command is what
// gets classified. ok is false when a wrapper is present but its inner command
// cannot be determined statically.
//
// A wrapper with no inner command resolves to a harmless stand-in: env prints
// the environment and xargs runs echo.
func unwrapWrappers(name string, args []string) (string, []string, bool, bool) {
	tainted := false
	for depth := 0; depth < 8; depth++ {
		var inner []string
		var known, wrapper bool
		switch name {
		case "env":
			wrapper = true
			var envTaint bool
			inner, known, envTaint = skipEnvOptions(args)
			tainted = tainted || envTaint
		case "nice":
			wrapper = true
			inner, known = skipNiceOptions(args)
		case "timeout":
			wrapper = true
			inner, known = skipTimeoutOptions(args)
		case "nohup", "builtin", "exec":
			wrapper = true
			inner, known = args, true
		case "command":
			wrapper = true
			inner, known = skipCommandOptions(args)
		case "xargs":
			wrapper = true
			inner, known = skipXargsOptions(args)
		}
		if !wrapper {
			return name, args, true, tainted
		}
		if !known {
			return "", nil, false, tainted
		}
		if len(inner) == 0 {
			switch name {
			case "xargs":
				return "echo", nil, true, tainted
			case "env":
				return "env", nil, true, tainted
			default:
				return "", nil, false, tainted
			}
		}
		if inner[0] == "" {
			return "", nil, false, tainted
		}
		name, args = inner[0], inner[1:]
	}
	return "", nil, false, tainted
}

func skipEnvOptions(args []string) ([]string, bool, bool) {
	tainted := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "":
			return nil, false, tainted
		case a == "--":
			return args[i+1:], true, tainted
		case a == "-u" || a == "--unset" || a == "-C" || a == "--chdir":
			i++ // takes a value
		case a == "-S" || strings.HasPrefix(a, "-S") || strings.HasPrefix(a, "--split-string"):
			return nil, false, tainted // runs a command string we cannot see
		case a == "-i" || a == "-0" || a == "-v" || a == "--ignore-environment" || a == "--null" || a == "--debug":
		case strings.HasPrefix(a, "--unset=") || strings.HasPrefix(a, "--chdir=") || strings.HasPrefix(a, "-u") && len(a) > 2:
		case strings.HasPrefix(a, "-"):
			return nil, false, tainted
		case strings.Contains(a, "="):
			// NAME=value assignment: may change what runs (LD_PRELOAD, PATH, GIT_PAGER).
			tainted = true
		default:
			return args[i:], true, tainted
		}
	}
	return nil, true, tainted
}

func skipNiceOptions(args []string) ([]string, bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "":
			return nil, false
		case a == "--":
			return args[i+1:], true
		case a == "-n" || a == "--adjustment":
			i++
		case strings.HasPrefix(a, "-n") || strings.HasPrefix(a, "--adjustment=") || isNumericOption(a):
		case strings.HasPrefix(a, "-"):
			return nil, false
		default:
			return args[i:], true
		}
	}
	return nil, true
}

func isNumericOption(a string) bool {
	if len(a) < 2 || a[0] != '-' {
		return false
	}
	for _, r := range a[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func skipTimeoutOptions(args []string) ([]string, bool) {
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "":
			return nil, false
		case a == "--":
			i++
		case a == "-s" || a == "-k" || a == "--signal" || a == "--kill-after":
			i++
			continue
		case a == "-v" || a == "--verbose" || a == "--foreground" || a == "--preserve-status" ||
			strings.HasPrefix(a, "--signal=") || strings.HasPrefix(a, "--kill-after=") ||
			strings.HasPrefix(a, "-s") || strings.HasPrefix(a, "-k"):
			continue
		case strings.HasPrefix(a, "-"):
			return nil, false
		}
		break
	}
	// The next word is the duration.
	if i >= len(args) {
		return nil, true
	}
	if args[i] == "" {
		return nil, false
	}
	return args[i+1:], true
}

func skipCommandOptions(args []string) ([]string, bool) {
	for i, a := range args {
		switch {
		case a == "-v" || a == "-V":
			return []string{"which"}, true // lookup only
		case a == "-p":
		case a == "--":
			return args[i+1:], true
		case strings.HasPrefix(a, "-"):
			return nil, false
		default:
			return args[i:], true
		}
	}
	return nil, true
}

func skipXargsOptions(args []string) ([]string, bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "":
			return nil, false
		case a == "--":
			return args[i+1:], true
		case a == "-n" || a == "-I" || a == "-P" || a == "-d" || a == "-L" || a == "-s" || a == "-E" || a == "-a" || a == "-i":
			i++ // takes a separate value
		case strings.HasPrefix(a, "-"):
			// flags, or options with an attached value (-n1, --max-args=1)
		default:
			return args[i:], true
		}
	}
	return nil, true
}
