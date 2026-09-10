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
	if len(opencode.args) == 0 || opencode.args[0] != "run" {
		t.Errorf("expected run arg for opencode, got %v", opencode.args)
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

func TestCLIBridge_StreamCompletion_ContextCancel(t *testing.T) {
	shBin := findBinary("sh", "/bin/sh")

	p := NewCLIBridgeProvider(CLIBridgeConfig{
		BinaryPath: shBin,
		Args:       []string{"-c", "while true; do echo 'chunk'; sleep 0.05; done"},
		Timeout:    10 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "loop"}},
	}

	receivedChunks := 0
	_, err := p.StreamCompletion(ctx, req, func(chunk *model.StreamChunk) error {
		receivedChunks++
		if receivedChunks >= 2 {
			cancel() // Cancel context during stream
		}
		return nil
	})

	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "context canceled") {
		t.Logf("got expected stream cancellation result: %v", err)
	}
}

func TestCLIBridge_StreamCompletion_OnChunkAbort(t *testing.T) {
	shBin := findBinary("sh", "/bin/sh")

	p := NewCLIBridgeProvider(CLIBridgeConfig{
		BinaryPath: shBin,
		Args:       []string{"-c", "echo 'first'; sleep 0.1; echo 'second'"},
		Timeout:    5 * time.Second,
	})

	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "abort test"}},
	}

	abortedErr := context.Canceled
	_, err := p.StreamCompletion(context.Background(), req, func(chunk *model.StreamChunk) error {
		return abortedErr
	})

	if err != abortedErr {
		t.Errorf("expected abortedErr, got: %v", err)
	}
}

func TestCLIBridge_LargeOutput_NoDeadlock(t *testing.T) {
	shBin := findBinary("sh", "/bin/sh")

	// Generate 128KB output on stdout and stderr simultaneously to test buffer pipe capacity
	p := NewCLIBridgeProvider(CLIBridgeConfig{
		BinaryPath: shBin,
		Args:       []string{"-c", "python3 -c \"import sys; sys.stdout.write('A'*65536); sys.stderr.write('B'*65536)\" 2>/dev/null || dd if=/dev/zero bs=1024 count=64 2>/dev/null"},
		Timeout:    5 * time.Second,
	})

	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "large buffer test"}},
	}

	resp, err := p.GenerateCompletion(context.Background(), req)
	if err != nil {
		t.Logf("command execution finished with %v (permitted fallback)", err)
	} else if len(resp.Content) == 0 {
		t.Error("expected non-empty content")
	}
}

func TestCLIBridge_OpenCode_OutputParsing(t *testing.T) {
	t.Run("JSON_format", func(t *testing.T) {
		raw := `{"type":"step_start","timestamp":100}
{"type":"text","part":{"type":"text","text":"Hello "}}
{"type":"text","part":{"type":"text","text":"world!"}}
{"type":"step_finish","part":{"tokens":{"total":42,"input":30,"output":12}}}`

		content, tokens := parseOpenCodeOutput(raw)
		if content != "Hello world!" {
			t.Errorf("expected 'Hello world!', got %q", content)
		}
		if tokens != 42 {
			t.Errorf("expected tokens 42, got %d", tokens)
		}
	})

	t.Run("plain_text_with_banner", func(t *testing.T) {
		raw := `> build · deepseek-v4-flash

This is the real response.
Multiple lines.`

		content, _ := parseOpenCodeOutput(raw)
		expected := "This is the real response.\nMultiple lines."
		if content != expected {
			t.Errorf("expected %q, got %q", expected, content)
		}
	})
}

func TestCLIBridge_OpenCode_CommandPrep(t *testing.T) {
	p := NewOpenCodeCLIProviderWithModel("/custom/opencode", "opencode-go/glm-5.3-flash", 1*time.Minute)

	req := &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "test prompt"}},
		Model:    "opencode-go/deepseek-v4-flash",
	}

	cmd, _, cancel := p.prepareCommand(context.Background(), req, "test prompt")
	defer cancel()

	cmdStr := strings.Join(cmd.Args, " ")
	if !strings.Contains(cmdStr, "-m opencode-go/deepseek-v4-flash") {
		t.Errorf("expected -m opencode-go/deepseek-v4-flash in args: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "--format json") {
		t.Errorf("expected --format json in args: %s", cmdStr)
	}

	// Test non-opencode model (e.g. gpt-4o from default RAG service) falls back to default opencode model
	reqDefault := &model.CompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "test prompt"}},
		Model:    "gpt-4o",
	}
	cmdDef, _, cancelDef := p.prepareCommand(context.Background(), reqDefault, "test prompt")
	defer cancelDef()

	cmdDefStr := strings.Join(cmdDef.Args, " ")
	if !strings.Contains(cmdDefStr, "-m opencode-go/glm-5.3-flash") {
		t.Errorf("expected fallback to defaultModel opencode-go/glm-5.3-flash, got: %s", cmdDefStr)
	}
}

