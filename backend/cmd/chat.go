// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/client"
	"github.com/qorvenai/qorven/cmd/tui"
	"github.com/qorvenai/qorven/internal/llm"
)

func init() {
	var (
		message  string
		session  string
		agent    string
		noStream bool
		useTUI   bool
	)

	chatCmd := &cobra.Command{
		Use:   "chat [agent]",
		Short: "Chat with an agent",
		Long: `Chat with an agent interactively or send a single message.

Interactive:
  qorven chat
  qorven chat researcher

Single-shot:
  qorven chat -m "What time is it?"
  qorven chat -m @prompt.txt

Pipe:
  echo "Analyze this" | qorven chat`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				agent = args[0]
			}
			return runChat(agent, message, session, noStream, useTUI)
		},
	}
	chatCmd.Flags().StringVarP(&message, "message", "m", "", "One-shot message (or @file)")
	chatCmd.Flags().StringVarP(&session, "session", "s", "", "Session ID to continue")
	chatCmd.Flags().StringVarP(&agent, "agent", "a", "", "Agent key")
	chatCmd.Flags().BoolVar(&noStream, "no-stream", false, "Disable streaming")
	chatCmd.Flags().BoolVar(&useTUI, "tui", false, "Use full TUI with sidebar")
	rootCmd.AddCommand(chatCmd)
}

func runChat(agentKey, message, sessionID string, noStream bool, useTUI bool) error {
	c, err := newHTTP()
	if err != nil {
		return err
	}

	// Resolve agent
	agentID, agentName, agentModel, err := resolveAgent(c, agentKey)
	if err != nil {
		return err
	}

	// Create session if needed
	if sessionID == "" {
		sid, err := createSession(c, agentID)
		if err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		sessionID = sid
	}

	// Piped stdin
	if message == "" && !tui.IsInteractive() {
		data, err := io.ReadAll(os.Stdin)
		if err == nil && len(data) > 0 {
			message = strings.TrimSpace(string(data))
		}
	}

	// @file support
	if strings.HasPrefix(message, "@") {
		content, err := tui.ReadContent(message)
		if err != nil {
			return err
		}
		message = content
	}

	// Single-shot
	if message != "" {
		return streamChatMessage(c, agentID, sessionID, message, noStream)
	}

	// Flush any pending terminal escape responses from stdin
	if tui.IsInteractive() {
		flushStdinNonblock()
	}

	// TUI mode — full terminal UI with sidebar (default for interactive)
	if (useTUI || (message == "" && tui.IsInteractive())) && !noStream {
		return tui.Run(agentName, agentID, agentModel, sessionID)
	}

	// Interactive - plain output, no escape sequences
	fmt.Printf("Connected to agent: %s\n", agentName)
	fmt.Printf("Type your message and press Enter. Use /exit to quit.\n")
	fmt.Println()
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		switch input {
		case "/exit", "/quit":
			fmt.Println("Goodbye!")
			return nil
		case "/new":
			sid, err := createSession(c, agentID)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			sessionID = sid
			fmt.Printf("[new session: %s]\n\n", sessionID[:8])
			continue
		case "/clear":
			fmt.Println()
	fmt.Println()
			fmt.Printf(" Session %s | /exit /new /clear /help\n", sessionID[:8])
			continue
		case "/help":
			fmt.Println("  /exit   - Quit")
			fmt.Println("  /new    - New session")
			fmt.Println("  /clear  - Clear screen")
			fmt.Println("  /help   - Show commands")
			fmt.Println()
			continue
		}

		if err := streamChatMessage(c, agentID, sessionID, input, false); err != nil {
			fmt.Printf("Error: %s\n", err)
		}
		fmt.Print("\n\n")
	}
	return nil
}

func resolveAgent(c *client.HTTPClient, agentKey string) (id, name, model string, err error) {
	resp, err := c.Get("/v1/agents")
	if err != nil {
		return "", "", "", err
	}
	var data struct {
		Agents []struct {
			ID          string `json:"id"`
			AgentKey    string `json:"agent_key"`
			DisplayName string `json:"display_name"`
			Model       string `json:"model"`
		} `json:"agents"`
	}
	if json.Unmarshal(resp, &data) != nil || len(data.Agents) == 0 {
		return "", "", "", fmt.Errorf("no agents configured - run: qorven init")
	}
	for _, a := range data.Agents {
		if strings.HasPrefix(a.AgentKey, "__") {
			continue // skip system agents
		}
		if agentKey == "" || a.AgentKey == agentKey || a.ID == agentKey {
			n := a.DisplayName
			if n == "" {
				n = a.AgentKey
			}
			return a.ID, n, llm.GetModelName(a.Model), nil
		}
	}
	return "", "", "", fmt.Errorf("agent %q not found", agentKey)
}

func createSession(c *client.HTTPClient, agentID string) (string, error) {
	resp, err := c.Post("/v1/sessions", map[string]any{
		"agent_id": agentID,
		"channel":  "cli",
	})
	if err != nil {
		return "", err
	}
	var data struct {
		ID string `json:"id"`
	}
	json.Unmarshal(resp, &data)
	if data.ID == "" {
		return "", fmt.Errorf("no session ID returned")
	}
	return data.ID, nil
}

func streamChatMessage(c *client.HTTPClient, agentID, sessionID, message string, noStream bool) error {
	body := map[string]any{
		"session_id": sessionID,
		"agent_id":   agentID,
		"message":    message,
		"stream":     !noStream,
	}

	if noStream {
		resp, err := c.Post("/v1/chat/completions", body)
		if err != nil {
			return err
		}
		var data struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		json.Unmarshal(resp, &data)
		if len(data.Choices) > 0 {
			fmt.Println(data.Choices[0].Message.Content)
		}
		return nil
	}

	// Streaming via SSE
	resp, err := c.PostRaw("/v1/chat/completions", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var event map[string]any
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}

		// OpenAI-format chunks
		if choices, ok := event["choices"].([]any); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]any); ok {
				if delta, ok := choice["delta"].(map[string]any); ok {
					if content, ok := delta["content"].(string); ok && content != "" {
						fmt.Print(content)
					}
				}
			}
		}

		// Tool events
		if evtType, ok := event["type"].(string); ok {
			switch evtType {
			case "tool_start":
				// Only show if data has a name
				if d, ok := event["data"].(map[string]any); ok {
					name, _ := d["name"].(string)
					if name != "" {
						fmt.Printf("\n[calling: %s] ", name)
					}
				}
			case "part":
				if d, ok := event["data"].(map[string]any); ok {
					partType, _ := d["type"].(string)
					if partType == "tool-call" {
						toolName, _ := d["toolName"].(string)
						if toolName != "" {
							fmt.Printf("\n[calling: %s] ", toolName)
						}
					} else if partType == "tool-result" {
						fmt.Print(".")
					}
				}
			case "tool_result":
				fmt.Print(".")
			}
		}
	}
	fmt.Println() // ensure output ends with newline
	return scanner.Err()
}
