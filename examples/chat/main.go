// Package main provides a simple example of streaming chat with Ollama.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/llm/ollama"
)

func main() {
	var (
		baseURL = flag.String("url", "http://localhost:11434", "Ollama base URL")
		model   = flag.String("model", "qwen3", "Model to use")
		prompt  = flag.String("prompt", "Hello! Tell me a short joke.", "Prompt to send")
	)
	flag.Parse()

	// Create client
	client := ollama.NewClient(*baseURL)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Prepare chat request
	req := &llm.ChatRequest{
		Model: *model,
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: *prompt,
			},
		},
		Stream:    true,
		KeepAlive: "5m",
	}

	fmt.Printf("Connecting to Ollama at %s...\n", *baseURL)
	fmt.Printf("Using model: %s\n", *model)
	fmt.Printf("Prompt: %s\n\n", *prompt)
	fmt.Println("Response:")
	fmt.Println("---")

	// Stream chat
	events, err := client.Chat(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting chat: %v\n", err)
		os.Exit(1)
	}

	// Wrap with watchdog
	config := llm.DefaultWatchdogConfig()
	config = config.WithIdleWarning(10*time.Second, func() {
		fmt.Fprintf(os.Stderr, "\n[Still waiting for response...]\n")
	})

	watchedEvents, cancelWatchdog := llm.WatchStream(ctx, events, config)
	defer cancelWatchdog()

	// Process events
	fullResponse := ""
	for event := range watchedEvents {
		if event.Message.Content != "" {
			fmt.Print(event.Message.Content)
			fullResponse += event.Message.Content
		}

		if event.Done {
			fmt.Println()
			fmt.Println("---")

			if event.DoneReason == "watchdog_timeout" {
				fmt.Fprintf(os.Stderr, "\n❌ Stream timed out (watchdog)\n")
				os.Exit(1)
			}

			if event.DoneReason != "" && event.DoneReason != "stop" {
				fmt.Printf("Done reason: %s\n", event.DoneReason)
			}

			// Print token statistics
			if event.PromptEvalCount > 0 {
				fmt.Printf("Prompt tokens: %d\n", event.PromptEvalCount)
			}
			if event.EvalCount > 0 {
				fmt.Printf("Response tokens: %d\n", event.EvalCount)
			}
			if event.TotalDuration > 0 {
				duration := time.Duration(event.TotalDuration)
				fmt.Printf("Total duration: %s\n", duration)

				if event.EvalCount > 0 {
					tokensPerSec := float64(event.EvalCount) / duration.Seconds()
					fmt.Printf("Tokens/sec: %.2f\n", tokensPerSec)
				}
			}
			break
		}
	}

	fmt.Println("\n✓ Chat complete")
}
