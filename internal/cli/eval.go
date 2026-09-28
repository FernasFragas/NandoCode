package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/eval"
	"github.com/spf13/cobra"
)

func newEvalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Run or validate coding-task evaluation fixtures",
	}
	cmd.AddCommand(newEvalRunCmd())
	cmd.AddCommand(newEvalValidateCmd())
	return cmd
}

func newEvalRunCmd() *cobra.Command {
	opts := eval.RunOptions{
		Provider:  eval.ProviderRecorded,
		Recording: "default",
		Jobs:      1,
		Repeat:    1,
	}
	cmd := &cobra.Command{
		Use:   "run [path]",
		Short: "Run evaluation fixtures",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "./evals"
			if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
				root = args[0]
			}
			if err := opts.Validate(); err != nil {
				return &cliExitError{code: 2, err: err}
			}
			fixtures, _, err := loadValidatedFixtures(root)
			if err != nil {
				return err
			}
			report, err := eval.Run(cmd.Context(), fixtures, opts, func(msg string) {
				fmt.Fprintln(cmd.ErrOrStderr(), msg)
			})
			if err != nil {
				return &cliExitError{code: 3, err: err}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Run: %s\n", report.RunID)
			fmt.Fprintf(cmd.OutOrStdout(), "Results: %s\n", report.Artifacts.ResultJSON)
			fmt.Fprintf(cmd.OutOrStdout(), "Report: %s\n", report.Artifacts.MarkdownReport)
			if opts.FailUnder > 0 && report.AggregateScore < opts.FailUnder {
				return &cliExitError{code: 1, err: fmt.Errorf("aggregate score %.2f below fail-under %.2f", report.AggregateScore, opts.FailUnder)}
			}
			if report.Counts.Errors > 0 {
				return &cliExitError{code: 3, err: fmt.Errorf("%d fixture(s) errored", report.Counts.Errors)}
			}
			if report.Counts.Failed > 0 {
				return &cliExitError{code: 1, err: fmt.Errorf("%d fixture(s) failed", report.Counts.Failed)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.Model, "model", "", "Model to use for live eval runs")
	cmd.Flags().StringVar((*string)(&opts.Provider), "provider", string(eval.ProviderRecorded), "Evaluation provider (recorded or live)")
	cmd.Flags().StringVar(&opts.OllamaURL, "ollama-url", "", "Ollama base URL for live eval runs")
	cmd.Flags().StringVar(&opts.Recording, "recording", "default", "Recording variant name")
	cmd.Flags().StringVar(&opts.OutputDir, "output-dir", ".tmp-evals", "Directory for eval artifacts")
	cmd.Flags().BoolVar(&opts.KeepWorkspaces, "keep-workspaces", false, "Retain temporary workspaces")
	cmd.Flags().IntVar(&opts.Jobs, "jobs", 1, "Number of fixtures to process concurrently")
	cmd.Flags().IntVar(&opts.Repeat, "repeat", 1, "Number of times to repeat each fixture")
	cmd.Flags().Float64Var(&opts.FailUnder, "fail-under", 0, "Minimum aggregate score threshold")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 0, "Optional per-fixture timeout override")
	return cmd
}

func newEvalValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate evaluation fixture structure and schema",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "./evals"
			if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
				root = args[0]
			}
			fixtures, resolvedRoot, err := loadValidatedFixtures(root)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Validated %d fixture(s) under %s\n", len(fixtures), resolvedRoot)
			for _, fixture := range fixtures {
				fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", fixture.ID)
			}
			return nil
		},
	}
}

func loadValidatedFixtures(root string) ([]eval.Fixture, string, error) {
	resolvedRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, "", &cliExitError{code: 2, err: fmt.Errorf("resolve eval path: %w", err)}
	}
	fixtures, err := eval.LoadAndValidate(resolvedRoot)
	if err != nil {
		return nil, "", &cliExitError{code: 2, err: err}
	}
	return fixtures, resolvedRoot, nil
}
