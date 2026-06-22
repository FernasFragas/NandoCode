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
			args := commandArgs(n)
			if isDestructiveCommand(name, args) {
				readOnly = false
				destructive = true
				return false
			}
			if isNeutralCommand(name) {
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

func commandArgs(call *syntax.CallExpr) []string {
	args := make([]string, 0, len(call.Args)-1)
	for _, word := range call.Args[1:] {
		if lit, ok := literalWord(word); ok {
			args = append(args, lit)
		} else {
			args = append(args, "")
		}
	}
	return args
}

func literalWord(word *syntax.Word) (string, bool) {
	var b strings.Builder
	for _, part := range word.Parts {
		lit, ok := part.(*syntax.Lit)
		if !ok {
			return "", false
		}
		b.WriteString(lit.Value)
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
	case "ls", "cat", "head", "tail", "wc", "sort", "uniq", "cut", "grep", "egrep", "fgrep", "rg", "find", "stat", "file", "du", "df", "env", "printenv", "which", "type", "date", "uname":
		return true
	case "git":
		return len(args) > 0 && readOnlyGitSubcommand(args[0])
	case "go":
		return len(args) > 0 && readOnlyGoSubcommand(args[0])
	default:
		return false
	}
}

func readOnlyGitSubcommand(sub string) bool {
	switch sub {
	case "status", "diff", "log", "show", "branch", "remote", "rev-parse", "ls-files", "grep":
		return true
	default:
		return false
	}
}

func readOnlyGoSubcommand(sub string) bool {
	switch sub {
	case "test", "list", "version", "env":
		return true
	default:
		return false
	}
}

func isDestructiveCommand(name string, args []string) bool {
	switch name {
	case "rm", "rmdir", "mv", "cp", "mkdir", "touch", "chmod", "chown", "dd", "tee", "truncate", "curl", "wget", "ssh", "scp", "sudo":
		return true
	case "git":
		return len(args) > 0 && !readOnlyGitSubcommand(args[0])
	case "go":
		return len(args) > 0 && !readOnlyGoSubcommand(args[0])
	default:
		return false
	}
}
