// Package builtin registers all built-in tools.
package builtin

import (
	"github.com/FernasFragas/nandocodego/internal/tools"
	"github.com/FernasFragas/nandocodego/internal/tools/bash"
	"github.com/FernasFragas/nandocodego/internal/tools/fileedit"
	"github.com/FernasFragas/nandocodego/internal/tools/fileread"
	"github.com/FernasFragas/nandocodego/internal/tools/filewrite"
	"github.com/FernasFragas/nandocodego/internal/tools/glob"
	"github.com/FernasFragas/nandocodego/internal/tools/grep"
	"github.com/FernasFragas/nandocodego/internal/tools/todo"
	"github.com/FernasFragas/nandocodego/internal/tools/webfetch"
)

// Register registers all built-in tools.
func Register(reg *tools.Registry) error {
	toRegister := []tools.Tool{
		bash.NewBashTool(),
		fileread.NewFileReadTool(),
		filewrite.NewFileWriteTool(),
		fileedit.NewFileEditTool(),
		glob.NewGlobTool(),
		grep.NewGrepTool(),
		webfetch.NewWebFetchTool(),
		todo.NewTodoWriteTool(),
		todo.NewTodoReadTool(),
	}
	for _, t := range toRegister {
		if err := reg.Register(t); err != nil {
			return err
		}
	}
	return nil
}

// NewRegistry returns a registry containing all built-in tools.
func NewRegistry() (*tools.Registry, error) {
	reg := tools.NewRegistry()
	if err := Register(reg); err != nil {
		return nil, err
	}
	return reg, nil
}

// NewRegistryWithTools returns default built-ins plus additional opt-in tools.
func NewRegistryWithTools(extra ...tools.Tool) (*tools.Registry, error) {
	reg, err := NewRegistry()
	if err != nil {
		return nil, err
	}
	for _, t := range extra {
		if t == nil {
			continue
		}
		if err := reg.Register(t); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

// NewReadOnlyRegistry returns a registry with read-only/discovery built-ins.
func NewReadOnlyRegistry() (*tools.Registry, error) {
	reg := tools.NewRegistry()
	toRegister := []tools.Tool{
		fileread.NewFileReadTool(),
		glob.NewGlobTool(),
		grep.NewGrepTool(),
	}
	for _, t := range toRegister {
		if err := reg.Register(t); err != nil {
			return nil, err
		}
	}
	return reg, nil
}
