package provider

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/your-org/contextforge/internal/model"
)

func findBinary(names ...string) string {
	for _, n := range names {
		if path, err := exec.LookPath(n); err == nil {
			return path
		}
	}
	return names[0]
}

func TestCLIBridge_GenerateCompletion_Stdin(t *testing.T) {
	catBin := findBinary("cat", "/bin/cat")

	p := NewCLIBridgeProvider(CLIBridgeConfig{
		BinaryPath:         catBin,
		PassPromptViaStdin: true,
		Timeout:            5 * time.Second,
	})

	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{
			{Role: model.RoleUser, Content: "Hello CLI Bridge Stdin"},
		},
	}

	resp, err := p.GenerateCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateCompletion failed: %v", err)
	}

	if resp.Content != "Hello CLI Bridge Stdin" {
		t.Errorf("expected 'Hello CLI Bridge Stdin', got %q", resp.Content)
	}
	if resp.Model != catBin {
		t.Errorf("expected model %q, got %q", catBin, resp.Model)
	}
}

func TestCLIBridge_GenerateCompletion_Args(t *testing.T) {
	echoBin := findBinary("echo", "/bin/echo")

	p := NewCLIBridgeProvider(CLIBridgeConfig{
		BinaryPath:         echoBin,
		PassPromptViaStdin: false,
		Timeout:            5 * time.Second,
	})

	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{
			{Role: model.RoleUser, Content: "Hello CLI Bridge Args"},
		},
	}

	resp, err := p.GenerateCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateCompletion failed: %v", err)
	}

	if strings.TrimSpace(resp.Content) != "Hello CLI Bridge Args" {
		t.Errorf("expected 'Hello CLI Bridge Args', got %q", resp.Content)
	}
}

func TestCLIBridge_StreamCompletion(t *testing.T) {
	catBin := findBinary("cat", "/bin/cat")

	p := NewCLIBridgeProvider(CLIBridgeConfig{
		BinaryPath:         catBin,
		PassPromptViaStdin: true,
		Timeout:            5 * time.Second,
	})

	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{
			{Role: model.RoleUser, Content: "Streaming test output"},
		},
	}

	var chunks []string
	var doneReceived bool

	resp, err := p.StreamCompletion(context.Background(), req, func(chunk *model.StreamChunk) error {
		if chunk.Done {
			doneReceived = true
		} else {
			chunks = append(chunks, chunk.Delta)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("StreamCompletion failed: %v", err)
	}
	if !doneReceived {
		t.Error("expected done=true chunk")
	}
	if strings.TrimSpace(resp.Content) != "Streaming test output" {
		t.Errorf("expected 'Streaming test output', got %q", resp.Content)
	}
	if strings.Join(chunks, "") != "Streaming test output" {
		t.Errorf("expected joined chunks 'Streaming test output', got %q", strings.Join(chunks, ""))
	}
}

func TestCLIBridge_NonZeroExitCode(t *testing.T) {
	shBin := findBinary("sh", "/bin/sh")

	p := NewCLIBridgeProvider(CLIBridgeConfig{
		BinaryPath: shBin,
		Args:       []string{"-c", "echo 'something went horribly wrong' >&2; exit 42"},
		Timeout:    5 * time.Second,
	})

	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "fail"}},
	}

	_, err := p.GenerateCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on non-zero exit code, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "42") {
		t.Errorf("expected error to mention exit code 42, got: %s", errStr)
	}
	if !strings.Contains(errStr, "something went horribly wrong") {
		t.Errorf("expected error to capture stderr, got: %s", errStr)
	}
}

func TestCLIBridge_TimeoutGuard(t *testing.T) {
	shBin := findBinary("sh", "/bin/sh")

	p := NewCLIBridgeProvider(CLIBridgeConfig{
		BinaryPath: shBin,
		Args:       []string{"-c", "sleep 5"},
		Timeout:    50 * time.Millisecond,
	})

	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "sleep"}},
	}

	start := time.Now()
	_, err := p.GenerateCompletion(context.Background(), req)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout message, got: %v", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout guard took too long to terminate process: %v", elapsed)
	}
}

func TestCLIBridge_ValidateBinary(t *testing.T) {
	valid := NewCLIBridgeProvider(CLIBridgeConfig{BinaryPath: "echo"})
	if err := valid.ValidateBinary(); err != nil {
		t.Errorf("expected valid binary to pass, got error: %v", err)
	}

	invalid := NewCLIBridgeProvider(CLIBridgeConfig{BinaryPath: "non_existent_binary_xyz_12345"})
	if err := invalid.ValidateBinary(); err == nil {
		t.Error("expected missing binary to return error, got nil")
	}
}

func TestDefaultPromptFormatter(t *testing.T) {
	t.Run("single message", func(t *testing.T) {
		req := &model.CompletionRequest{
			Messages: []model.ChatMessage{{Role: "user", Content: "Single prompt"}},
		}
		if res := DefaultPromptFormatter(req); res != "Single prompt" {
			t.Errorf("expected 'Single prompt', got %q", res)
		}
	})

	t.Run("multi-turn conversation", func(t *testing.T) {
		req := &model.CompletionRequest{
			Messages: []model.ChatMessage{
				{Role: model.RoleSystem, Content: "System prompt"},
				{Role: model.RoleUser, Content: "User query"},
				{Role: model.RoleAssistant, Content: "Assistant answer"},
			},
		}
		formatted := DefaultPromptFormatter(req)
		if !strings.Contains(formatted, "[System]: System prompt") {
			t.Errorf("expected System line in prompt: %s", formatted)
		}
		if !strings.Contains(formatted, "[User]: User query") {
			t.Errorf("expected User line in prompt: %s", formatted)
		}
		if !strings.Contains(formatted, "[Assistant]: Assistant answer") {
			t.Errorf("expected Assistant line in prompt: %s", formatted)
		}
	})
}

func TestCLIBridge_Presets(t *testing.T) {
	opencode := NewOpenCodeCLIProvider("", 1*time.Minute)
	if opencode.binaryPath != "opencode" {
		t.Errorf("expected opencode, got %s", opencode.binaryPath)
	}
	if !opencode.passPromptViaStdin {
		t.Error("expected passPromptViaStdin to be true")
	}

	claude := NewClaudeCLIProvider("", 1*time.Minute)
	if claude.binaryPath != "claude" {
		t.Errorf("expected claude, got %s", claude.binaryPath)
	}
	if len(claude.args) == 0 || claude.args[0] != "-p" {
		t.Errorf("expected -p arg for claude, got %v", claude.args)
	}

	gemini := NewGeminiCLIProvider("", 1*time.Minute)
	if gemini.binaryPath != "gemini" {
		t.Errorf("expected gemini, got %s", gemini.binaryPath)
	}

	codex := NewCodexCLIProvider("", 1*time.Minute)
	if codex.binaryPath != "codex" {
		t.Errorf("expected codex, got %s", codex.binaryPath)
	}
}
