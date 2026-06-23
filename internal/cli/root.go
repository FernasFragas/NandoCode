// Package cli provides the command-line interface for nandocodego.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/FernasFragas/Nandocode/internal/logging"
	"github.com/FernasFragas/Nandocode/internal/version"
	"github.com/spf13/cobra"
)

var runPrintFn = runPrint

// NewRootCmd creates the root command for the CLI.
func NewRootCmd() *cobra.Command {
	return newRootCommand(context.Background(), os.Stdout, os.Stderr)
}

func newRootCommand(ctx context.Context, out, errOut io.Writer) *cobra.Command {
	opts := struct {
		logLevel                  string
		logFormat                 string
		model                     string
		ollamaURL                 string
		noAltScreen               bool
		printInput                string
		jsonOutput                bool
		numCtx                    int
		llmStreamIdleTimeout      string
		cloudLLMStreamIdleTimeout string
	}{
		logLevel: "info",
	}

	rootCmd := &cobra.Command{
		Use:   "nandocodego",
		Short: "Local-first AI coding assistant powered by Ollama",
		Long: `nandocodego is a Go-based agentic coding CLI that brings the power of 
large language models to your development workflow using local LLMs via Ollama.

Features:
  • Local-first: Your code never leaves your machine
  • Rich terminal UI with streaming responses
  • Powerful agentic capabilities (sub-agents, memory, hooks)
  • Comprehensive permission system
  • MCP (Model Context Protocol) integration`,
		Version: version.Version,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			level := logging.ParseLevel(opts.logLevel)
			format := logging.Format(opts.logFormat)
			logger := logging.NewWithWriter(cmd.ErrOrStderr(), level, format)
			cmd.SetContext(context.WithValue(cmd.Context(), loggerContextKey{}, logger))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.printInput != "" {
				return runPrintFn(cmd.Context(), cmd, printOptions{
					input:                     opts.printInput,
					jsonOutput:                opts.jsonOutput,
					model:                     opts.model,
					ollamaURL:                 opts.ollamaURL,
					numCtx:                    opts.numCtx,
					llmStreamIdleTimeout:      opts.llmStreamIdleTimeout,
					cloudLLMStreamIdleTimeout: opts.cloudLLMStreamIdleTimeout,
				})
			}
			// Check if this is a subcommand being called
			if len(args) > 0 {
				return cmd.Help()
			}
			// No args: run REPL
			return runREPL(cmd.Context(), cmd, replOptions{
				model:                     opts.model,
				ollamaURL:                 opts.ollamaURL,
				noAltScreen:               opts.noAltScreen,
				numCtx:                    opts.numCtx,
				llmStreamIdleTimeout:      opts.llmStreamIdleTimeout,
				cloudLLMStreamIdleTimeout: opts.cloudLLMStreamIdleTimeout,
			})
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rootCmd.SetContext(ctx)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&opts.logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&opts.logFormat, "log-format", "", "Log format (text, json, auto)")

	// REPL-specific flags
	rootCmd.Flags().StringVar(&opts.model, "model", "", "Default LLM model to use")
	rootCmd.Flags().StringVar(&opts.ollamaURL, "ollama-url", "", "Ollama base URL")
	rootCmd.Flags().BoolVar(&opts.noAltScreen, "no-alt-screen", false, "Disable alternate screen (useful for testing)")
	rootCmd.Flags().StringVar(&opts.printInput, "print", "", "Run a single prompt non-interactively and print the result")
	rootCmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "When used with --print, output JSON")
	rootCmd.Flags().IntVar(&opts.numCtx, "num-ctx", 0, "Ollama context window size in tokens (0 = use default, default is 1000000)")
	rootCmd.Flags().StringVar(&opts.llmStreamIdleTimeout, "llm-stream-idle-timeout", "", "LLM stream idle watchdog timeout")
	rootCmd.Flags().StringVar(&opts.cloudLLMStreamIdleTimeout, "cloud-llm-stream-idle-timeout", "", "Ollama Cloud stream idle watchdog timeout")

	// Version flag (cobra handles this automatically with Version field)
	rootCmd.SetVersionTemplate(version.Info() + "\n")

	// Add subcommands
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newIndexCmd())
	rootCmd.AddCommand(newServerCmd())

	return rootCmd
}

type loggerContextKey struct{}

func loggerFromCommand(cmd *cobra.Command) *slog.Logger {
	if logger, ok := cmd.Context().Value(loggerContextKey{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version.Info())
			return nil
		},
	}
}

// Run executes the CLI with injectable context and args.
func Run(ctx context.Context, args []string) error {
	cmd := newRootCommand(ctx, os.Stdout, os.Stderr)
	cmd.SetArgs(args)
	return cmd.ExecuteContext(ctx)
}

// ExitCode maps command errors to process exit codes.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ec interface{ ExitCode() int }
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return 1
}

// Execute runs the root command.
func Execute() {
	if err := Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitCode(err))
	}
}
