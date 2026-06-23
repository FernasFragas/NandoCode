package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FernasFragas/Nandocode/internal/config"
	"github.com/FernasFragas/Nandocode/internal/paths"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create default config.toml in the user config directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd.Context(), cmd)
		},
	}
}

func runInit(ctx context.Context, cmd *cobra.Command) error {
	_ = ctx
	dir := paths.ConfigDir()
	target := filepath.Join(dir, "config.toml")
	if _, err := os.Stat(target); err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Config already exists at %s\n", target)
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, []byte(config.DefaultConfigTOML()), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created config at %s\n", target)
	return nil
}
