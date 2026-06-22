package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FernasFragas/nandocodego/internal/config"
	"github.com/FernasFragas/nandocodego/internal/llm/ollama"
	"github.com/FernasFragas/nandocodego/internal/paths"
	"github.com/FernasFragas/nandocodego/internal/semantic"
	"github.com/spf13/cobra"
)

func newIndexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Manage local semantic workspace index",
	}

	cmd.AddCommand(newIndexBuildCmd())
	cmd.AddCommand(newIndexRefreshCmd())
	cmd.AddCommand(newIndexStatusCmd())
	cmd.AddCommand(newIndexClearCmd())
	return cmd
}

func newIndexBuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "build [path]",
		Short: "Build semantic index for workspace path (default: cwd)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, svc, cfg, err := indexSetup(args)
			if err != nil {
				return err
			}
			report, err := svc.Build(cmd.Context(), semantic.BuildRequest{Root: root, Config: cfg})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Index build complete: files_seen=%d files_indexed=%d records=%d skipped=%d duration=%s\n",
				report.FilesSeen, report.FilesIndexed, report.RecordsIndexed, report.FilesSkipped, report.Duration.Round(time.Millisecond))
			return nil
		},
	}
}

func newIndexRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh [path]",
		Short: "Refresh semantic index for workspace path (default: cwd)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, svc, cfg, err := indexSetup(args)
			if err != nil {
				return err
			}
			report, err := svc.Refresh(cmd.Context(), semantic.RefreshRequest{
				Root:     root,
				Config:   cfg,
				MaxFiles: cfg.PromptRefreshMaxFiles,
				Timeout:  cfg.PromptRefreshTimeout,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Index refresh complete: files_seen=%d files_indexed=%d records=%d skipped=%d duration=%s\n",
				report.FilesSeen, report.FilesIndexed, report.RecordsIndexed, report.FilesSkipped, report.Duration.Round(time.Millisecond))
			return nil
		},
	}
}

func newIndexStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [path]",
		Short: "Show semantic index status for workspace path (default: cwd)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, svc, cfg, err := indexSetup(args)
			if err != nil {
				return err
			}
			st, err := svc.Status(cmd.Context(), root)
			if err != nil {
				return err
			}
			model := st.Model
			if strings.TrimSpace(model) == "" {
				model = cfg.Model
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Index status: exists=%t compatible=%t model=%s dims=%d records=%d files=%d updated=%s\n",
				st.Exists, st.Compatible, model, st.Dimensions, st.RecordCount, st.FileCount, st.UpdatedAt.Format(time.RFC3339))
			return nil
		},
	}
}

func newIndexClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear [path]",
		Short: "Clear semantic index for workspace path (default: cwd)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, svc, _, err := indexSetup(args)
			if err != nil {
				return err
			}
			if err := svc.Clear(cmd.Context(), root); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Index cleared")
			return nil
		},
	}
}

func indexSetup(args []string) (string, *semantic.LocalService, semantic.Config, error) {
	root := "."
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		root = args[0]
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", nil, semantic.Config{}, err
	}
	cfgRes, cfgErr := config.Load(
		filepath.Join(paths.ConfigDir(), "config.toml"),
		filepath.Join(wd, ".nandocodego", "config.toml"),
		config.FlagOverrides{},
	)
	cfg := semantic.DefaultConfig()
	ollamaURL := "http://localhost:11434"
	if cfgErr == nil {
		cfg = cfgRes.Config.SemanticIndex
		ollamaURL = cfgRes.Config.OllamaBaseURL
	}

	client := ollama.NewClient(ollamaURL)
	store := semantic.NewLocalStore(paths.CacheDir())
	svc := semantic.NewLocalService(store, semantic.LLMEmbedder{Client: client})
	canonical, err := semantic.CanonicalRoot(root)
	if err != nil {
		return "", nil, semantic.Config{}, err
	}
	return canonical, svc, cfg, nil
}

// compile-time check: keep context imported for cobra commands using contexts.
var _ = context.Background
