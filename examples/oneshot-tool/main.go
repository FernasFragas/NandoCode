// Command oneshot-tool demonstrates Phase 3 tools without the agent loop.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/FernasFragas/nandocodego/internal/tools"
	"github.com/FernasFragas/nandocodego/internal/tools/bash"
	"github.com/FernasFragas/nandocodego/internal/tools/builtin"
	"github.com/FernasFragas/nandocodego/internal/tools/fileread"
	"github.com/FernasFragas/nandocodego/internal/tools/filewrite"
)

func main() {
	dir, err := os.MkdirTemp("", "nandocodego-tools-*")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(dir)

	ctx := tools.DefaultContext(context.Background(), dir)
	reg, err := builtin.NewRegistry()
	if err != nil {
		fatal(err)
	}

	fmt.Printf("working_dir=%s\n", dir)
	fmt.Printf("tools=%d\n", len(reg.All()))

	writeTool, _ := reg.Lookup("FileWrite")
	writeInput := filewrite.Input{Path: "hello.txt", Content: "hello from Phase 3\n"}
	if perm := writeTool.CheckPermissions(ctx, writeInput); perm.Decision != tools.PermAsk {
		fatal(fmt.Errorf("expected FileWrite to ask, got %s", perm.Decision))
	}
	ctx.PermissionMode = tools.PermissionBypassPermissions
	writeResult, err := writeTool.Call(ctx, writeInput, nil)
	if err != nil {
		fatal(err)
	}
	fmt.Println(writeResult.Display)

	readTool, _ := reg.Lookup("FileRead")
	readResult, err := readTool.Call(ctx, fileread.Input{Path: "hello.txt"}, nil)
	if err != nil {
		fatal(err)
	}
	readOut := readResult.Data.(fileread.Output)
	fmt.Printf("read=%q\n", readOut.Content)

	bashTool, _ := reg.Lookup("Bash")
	lsInput := bash.Input{Command: "ls"}
	if perm := bashTool.CheckPermissions(ctx, lsInput); perm.Decision != tools.PermAllow {
		fatal(fmt.Errorf("expected ls to be allowed, got %s", perm.Decision))
	}
	lsResult, err := bashTool.Call(ctx, lsInput, nil)
	if err != nil {
		fatal(err)
	}
	lsJSON, _ := json.Marshal(lsResult.Data)
	fmt.Printf("bash_ls=%s\n", lsJSON)

	ctx.PermissionMode = tools.PermissionDefault
	rmInput := bash.Input{Command: "rm -rf ."}
	rmPerm := bashTool.CheckPermissions(ctx, rmInput)
	fmt.Printf("rm_permission=%s\n", rmPerm.Decision)
	if rmPerm.Decision == tools.PermAllow {
		fatal(fmt.Errorf("unsafe command was allowed"))
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
