package llama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"little-golem/src/config"
)

// Server manages a spawned llama-server child process.
type Server struct {
	Cmd      *exec.Cmd
	Base     string
	LogFile  *os.File
	ToolDefs []map[string]any // OpenAI-style tool definitions sent with each request

	Done    chan struct{}
	ExitErr error
}

// StartServer picks a free localhost port, spawns llama-server pointed at
// the given GGUF, and returns a *Server talking to its HTTP API.
func StartServer(modelPath string) (*Server, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	logFile, err := os.CreateTemp("", "little-golem-llama-*.log")
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(config.LlamaBin,
		"--model", modelPath,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--ctx-size", strconv.Itoa(config.CtxSize),
		"--jinja",
		"--reasoning-format", "deepseek",
		"--reasoning-budget", strconv.Itoa(config.ThinkBudget),
		"--reasoning-budget-message", config.ThinkBudgetMessage,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"LD_LIBRARY_PATH="+config.LlamaLibDir+":"+os.Getenv("LD_LIBRARY_PATH"))
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("starting %s: %w", config.LlamaBin, err)
	}

	s := &Server{
		Cmd:     cmd,
		Base:    "http://127.0.0.1:" + strconv.Itoa(port),
		LogFile: logFile,
		Done:    make(chan struct{}),
	}
	go func() {
		s.ExitErr = cmd.Wait()
		close(s.Done)
	}()
	return s, nil
}

// Stop kills the llama-server process and waits for it to exit.
func (s *Server) Stop() {
	if s.Cmd != nil && s.Cmd.Process != nil {
		s.Cmd.Process.Kill()
		<-s.Done
	}
	if s.LogFile != nil {
		s.LogFile.Close()
	}
}

// WaitReady polls /health until the model is loaded or the process dies.
func (s *Server) WaitReady() error {
	deadline := time.Now().Add(LoadTimeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-s.Done:
			return fmt.Errorf("llama-server exited: %v (log: %s)", s.ExitErr, s.LogFile.Name())
		default:
		}
		resp, err := client.Get(s.Base + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for llama-server (log: %s)", s.LogFile.Name())
}

// Complete runs one non-streaming, tool-free completion and returns the
// reply text (reasoning excluded).
func (s *Server) Complete(ctx context.Context, msgs []ChatMessage, maxTokens int) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"messages":       msgs,
		"stream":         false,
		"temperature":    0.3,
		"repeat_penalty": 1.1,
		"max_tokens":     maxTokens,
		"cache_prompt":   true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return "", fmt.Errorf("llama-server: %s", strings.TrimSpace(string(raw)))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llama-server: empty response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// Stream posts the chat history and pushes decoded SSE chunks into out.
// The channel is closed when the stream ends, errors, or ctx is canceled.
func (s *Server) Stream(ctx context.Context, history []ChatMessage, out chan<- StreamEvent) {
	defer close(out)

	payload := map[string]any{
		"messages":       history,
		"stream":         true,
		"temperature":    0.6,
		"top_p":          0.95,
		"repeat_penalty": 1.1,
		"cache_prompt":   true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if len(s.ToolDefs) > 0 {
		payload["tools"] = s.ToolDefs
		payload["tool_choice"] = "auto"
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.Base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		out <- StreamEvent{Err: err}
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			out <- StreamEvent{Err: err}
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		out <- StreamEvent{Err: fmt.Errorf("llama-server: %s", strings.TrimSpace(string(raw)))}
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Reasoning string      `json:"reasoning_content"`
					Content   string      `json:"content"`
					ToolCalls []ToolChunk `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *Usage `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		// Final usage chunk (stream_options.include_usage) carries totals
		// and empty choices.
		if chunk.Usage != nil {
			select {
			case out <- StreamEvent{Usage: chunk.Usage}:
			case <-ctx.Done():
				return
			}
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		d := chunk.Choices[0].Delta
		if d.Reasoning == "" && d.Content == "" && len(d.ToolCalls) == 0 {
			continue
		}
		for _, tc := range d.ToolCalls {
			if tc.Function.Name == "" && tc.Function.Arguments == "" {
				continue
			}
			ev := StreamEvent{ToolDelta: &ToolDelta{
				Index: tc.Index,
				Name:  tc.Function.Name,
				Args:  tc.Function.Arguments,
			}}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
		if d.Reasoning == "" && d.Content == "" {
			continue
		}
		select {
		case out <- StreamEvent{Reasoning: d.Reasoning, Content: d.Content}:
		case <-ctx.Done():
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		out <- StreamEvent{Err: err}
	}
}
