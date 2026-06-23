package cli

import (
	"fmt"
	"time"

	"github.com/FernasFragas/Nandocode/internal/server"
	"github.com/spf13/cobra"
)

type serverOptions struct {
	bind                      string
	port                      int
	token                     string
	noUI                      bool
	model                     string
	ollamaURL                 string
	numCtx                    int
	llmStreamIdleTimeout      string
	cloudLLMStreamIdleTimeout string
	maxSessions               int
	idleTimeout               string
	readTimeout               string
	writeTimeout              string
}

func newServerCmd() *cobra.Command {
	opts := serverOptions{}
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run HTTP server with SSE interface",
		RunE: func(cmd *cobra.Command, _ []string) error {
			idle, err := parseDurationDefault(opts.idleTimeout, "30m")
			if err != nil {
				return err
			}
			read, err := parseDurationDefault(opts.readTimeout, "30s")
			if err != nil {
				return err
			}
			write, err := parseDurationDefault(opts.writeTimeout, "0")
			if err != nil {
				return err
			}
			return server.RunUntilSignal(cmd.Context(), loggerFromCommand(cmd), server.Config{
				Bind: opts.bind, Port: opts.port, Token: opts.token, NoUI: opts.noUI,
				Model: opts.model, OllamaURL: opts.ollamaURL, NumCtx: opts.numCtx,
				LLMStreamIdleTimeout: opts.llmStreamIdleTimeout, CloudLLMStreamIdleTimeout: opts.cloudLLMStreamIdleTimeout,
				MaxSessions: opts.maxSessions, IdleTimeout: idle, ReadTimeout: read, WriteTimeout: write,
			})
		},
	}
	cmd.Flags().StringVar(&opts.bind, "bind", "127.0.0.1", "Bind address")
	cmd.Flags().IntVar(&opts.port, "port", 8080, "Port")
	cmd.Flags().StringVar(&opts.token, "token", "", "Bearer token for API auth")
	cmd.Flags().BoolVar(&opts.noUI, "no-ui", false, "Disable embedded web UI")
	cmd.Flags().StringVar(&opts.model, "model", "", "Default LLM model")
	cmd.Flags().StringVar(&opts.ollamaURL, "ollama-url", "", "Ollama base URL")
	cmd.Flags().IntVar(&opts.numCtx, "num-ctx", 0, "Ollama context window size")
	cmd.Flags().StringVar(&opts.llmStreamIdleTimeout, "llm-stream-idle-timeout", "", "LLM stream idle watchdog timeout")
	cmd.Flags().StringVar(&opts.cloudLLMStreamIdleTimeout, "cloud-llm-stream-idle-timeout", "", "Ollama Cloud stream idle watchdog timeout")
	cmd.Flags().IntVar(&opts.maxSessions, "max-sessions", 10, "Max concurrent sessions")
	cmd.Flags().StringVar(&opts.idleTimeout, "idle-timeout", "30m", "Session idle timeout")
	cmd.Flags().StringVar(&opts.readTimeout, "read-timeout", "30s", "HTTP read timeout")
	cmd.Flags().StringVar(&opts.writeTimeout, "write-timeout", "0", "HTTP write timeout; 0 disables timeout for long SSE streams")
	return cmd
}

func parseDurationDefault(raw, fallback string) (time.Duration, error) {
	value := raw
	if value == "" {
		value = fallback
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", value, err)
	}
	return d, nil
}
