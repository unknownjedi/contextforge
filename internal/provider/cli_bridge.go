package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/your-org/contextforge/internal/model"
)

const (
	DefaultCLITimeout = 2 * time.Minute
)

// CLIBridgeConfig configures the native host CLI bridge provider.
type CLIBridgeConfig struct {
	// BinaryPath is the name or path of the host binary (e.g. "opencode", "claude", "gemini", "codex").
	BinaryPath string

	// Args contains default CLI arguments/flags before the prompt.
	Args []string

	// PassPromptViaStdin specifies whether the prompt is piped to stdin (true) or passed as a trailing argument (false).
	PassPromptViaStdin bool

	// Timeout specifies the maximum duration allowed for process execution.
	Timeout time.Duration

	// Env contains extra environment variables (KEY=VALUE) for the subprocess.
	Env []string

	// WorkDir specifies the working directory for the command.
	WorkDir string

	// PromptFormatter converts a CompletionRequest into the CLI prompt string.
	PromptFormatter func(req *model.CompletionRequest) string
}

// CLIBridgeProvider implements LLMProvider by executing host CLI binaries.
type CLIBridgeProvider struct {
	binaryPath         string
	args               []string
	passPromptViaStdin bool
	timeout            time.Duration
	env                []string
	workDir            string
	promptFormatter    func(req *model.CompletionRequest) string
}

// NewCLIBridgeProvider creates a new CLIBridgeProvider.
func NewCLIBridgeProvider(cfg CLIBridgeConfig) *CLIBridgeProvider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultCLITimeout
	}
	formatter := cfg.PromptFormatter
	if formatter == nil {
		formatter = DefaultPromptFormatter
	}

	return &CLIBridgeProvider{
		binaryPath:         cfg.BinaryPath,
		args:               cfg.Args,
		passPromptViaStdin: cfg.PassPromptViaStdin,
		timeout:            timeout,
		env:                cfg.Env,
		workDir:            cfg.WorkDir,
		promptFormatter:    formatter,
	}
}

// ValidateBinary checks if the configured host binary exists in PATH or at the absolute path.
func (p *CLIBridgeProvider) ValidateBinary() error {
	_, err := exec.LookPath(p.binaryPath)
	if err != nil {
		return fmt.Errorf("host CLI binary %q not found in PATH: %w", p.binaryPath, err)
	}
	return nil
}

// DefaultPromptFormatter converts messages into a structured conversational prompt.
func DefaultPromptFormatter(req *model.CompletionRequest) string {
	if req == nil || len(req.Messages) == 0 {
		return ""
	}
	if len(req.Messages) == 1 {
		return req.Messages[0].Content
	}

	var sb strings.Builder
	for _, msg := range req.Messages {
		switch strings.ToLower(msg.Role) {
		case model.RoleSystem:
			sb.WriteString(fmt.Sprintf("[System]: %s\n\n", msg.Content))
		case model.RoleUser:
			sb.WriteString(fmt.Sprintf("[User]: %s\n\n", msg.Content))
		case model.RoleAssistant:
			sb.WriteString(fmt.Sprintf("[Assistant]: %s\n\n", msg.Content))
		default:
			sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Role, msg.Content))
		}
	}
	return strings.TrimSpace(sb.String())
}

// killProcessGroup terminates the process and all child processes in its process group.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	if pid > 0 {
		// Send SIGKILL to the entire process group
		if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil {
			return nil
		}
	}
	return cmd.Process.Kill()
}

// prepareCommand constructs the exec.Cmd with process group attributes and timeout handling.
func (p *CLIBridgeProvider) prepareCommand(ctx context.Context, formattedPrompt string) (*exec.Cmd, context.Context, context.CancelFunc) {
	execCtx, cancel := context.WithTimeout(ctx, p.timeout)

	var cmdArgs []string
	cmdArgs = append(cmdArgs, p.args...)

	if !p.passPromptViaStdin && formattedPrompt != "" {
		cmdArgs = append(cmdArgs, formattedPrompt)
	}

	cmd := exec.CommandContext(execCtx, p.binaryPath, cmdArgs...)

	// Enable process group for clean subtree termination
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Register cancel function to kill process group
	cmd.Cancel = func() error {
		return killProcessGroup(cmd)
	}

	if len(p.env) > 0 {
		cmd.Env = append(cmd.Environ(), p.env...)
	}
	if p.workDir != "" {
		cmd.Dir = p.workDir
	}

	if p.passPromptViaStdin {
		cmd.Stdin = strings.NewReader(formattedPrompt)
	}

	return cmd, execCtx, cancel
}

// GenerateCompletion runs the CLI binary and captures stdout as the completion response.
func (p *CLIBridgeProvider) GenerateCompletion(ctx context.Context, req *model.CompletionRequest) (*model.CompletionResponse, error) {
	startTime := time.Now()

	formattedPrompt := p.promptFormatter(req)
	cmd, execCtx, cancel := p.prepareCommand(ctx, formattedPrompt)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	durationMs := time.Since(startTime).Milliseconds()

	if runErr != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("CLI binary %q timed out after %v: %w", p.binaryPath, p.timeout, execCtx.Err())
		}
		if execCtx.Err() == context.Canceled {
			return nil, fmt.Errorf("CLI binary %q canceled: %w", p.binaryPath, execCtx.Err())
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg == "" {
				errMsg = runErr.Error()
			}
			return nil, fmt.Errorf("CLI binary %q exited with code %d: %s", p.binaryPath, exitErr.ExitCode(), errMsg)
		}
		return nil, fmt.Errorf("CLI binary %q execution failed: %w (stderr: %s)", p.binaryPath, runErr, strings.TrimSpace(stderr.String()))
	}

	return &model.CompletionResponse{
		Content:    strings.TrimSpace(stdout.String()),
		Model:      p.binaryPath,
		DurationMs: durationMs,
	}, nil
}

// StreamCompletion runs the CLI binary and streams stdout chunks through the onChunk callback.
func (p *CLIBridgeProvider) StreamCompletion(ctx context.Context, req *model.CompletionRequest, onChunk func(chunk *model.StreamChunk) error) (*model.CompletionResponse, error) {
	startTime := time.Now()

	formattedPrompt := p.promptFormatter(req)
	cmd, execCtx, cancel := p.prepareCommand(ctx, formattedPrompt)
	defer cancel()

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stdout pipe for %q: %w", p.binaryPath, err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start CLI binary %q: %w", p.binaryPath, err)
	}

	var waited bool
	defer func() {
		if cmd.Process != nil && !waited {
			_ = killProcessGroup(cmd)
			_ = cmd.Wait()
		}
	}()

	var fullContent strings.Builder
	buf := make([]byte, 512)

	for {
		n, readErr := stdoutPipe.Read(buf)
		if n > 0 {
			chunkStr := string(buf[:n])
			fullContent.WriteString(chunkStr)

			if onChunk != nil {
				if err := onChunk(&model.StreamChunk{
					Delta: chunkStr,
					Done:  false,
				}); err != nil {
					waited = true
					_ = killProcessGroup(cmd)
					_ = cmd.Wait()
					return nil, err
				}
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			waited = true
			_ = killProcessGroup(cmd)
			_ = cmd.Wait()
			return nil, fmt.Errorf("error reading stdout from %q: %w", p.binaryPath, readErr)
		}
	}

	waited = true
	waitErr := cmd.Wait()
	durationMs := time.Since(startTime).Milliseconds()

	if waitErr != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("CLI binary %q timed out after %v: %w", p.binaryPath, p.timeout, execCtx.Err())
		}
		if execCtx.Err() == context.Canceled {
			return nil, fmt.Errorf("CLI binary %q canceled: %w", p.binaryPath, execCtx.Err())
		}
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg == "" {
				errMsg = waitErr.Error()
			}
			return nil, fmt.Errorf("CLI binary %q exited with code %d: %s", p.binaryPath, exitErr.ExitCode(), errMsg)
		}
		return nil, fmt.Errorf("CLI binary %q wait failed: %w (stderr: %s)", p.binaryPath, waitErr, strings.TrimSpace(stderr.String()))
	}

	if onChunk != nil {
		if err := onChunk(&model.StreamChunk{
			Delta: "",
			Done:  true,
		}); err != nil {
			return nil, err
		}
	}

	return &model.CompletionResponse{
		Content:    strings.TrimSpace(fullContent.String()),
		Model:      p.binaryPath,
		DurationMs: durationMs,
	}, nil
}

// Preset factory constructors for standard CLI tools.

// NewOpenCodeCLIProvider creates a CLI bridge configured for OpenCode CLI (`opencode`).
func NewOpenCodeCLIProvider(binaryPath string, timeout time.Duration) *CLIBridgeProvider {
	if binaryPath == "" {
		binaryPath = "opencode"
	}
	return NewCLIBridgeProvider(CLIBridgeConfig{
		BinaryPath:         binaryPath,
		Args:               []string{"prompt"},
		PassPromptViaStdin: true,
		Timeout:            timeout,
	})
}

// NewClaudeCLIProvider creates a CLI bridge configured for Claude Code (`claude`).
func NewClaudeCLIProvider(binaryPath string, timeout time.Duration) *CLIBridgeProvider {
	if binaryPath == "" {
		binaryPath = "claude"
	}
	return NewCLIBridgeProvider(CLIBridgeConfig{
		BinaryPath:         binaryPath,
		Args:               []string{"-p"},
		PassPromptViaStdin: true,
		Timeout:            timeout,
	})
}

// NewGeminiCLIProvider creates a CLI bridge configured for Gemini CLI (`gemini`).
func NewGeminiCLIProvider(binaryPath string, timeout time.Duration) *CLIBridgeProvider {
	if binaryPath == "" {
		binaryPath = "gemini"
	}
	return NewCLIBridgeProvider(CLIBridgeConfig{
		BinaryPath:         binaryPath,
		PassPromptViaStdin: true,
		Timeout:            timeout,
	})
}

// NewCodexCLIProvider creates a CLI bridge configured for Codex CLI (`codex`).
func NewCodexCLIProvider(binaryPath string, timeout time.Duration) *CLIBridgeProvider {
	if binaryPath == "" {
		binaryPath = "codex"
	}
	return NewCLIBridgeProvider(CLIBridgeConfig{
		BinaryPath:         binaryPath,
		PassPromptViaStdin: true,
		Timeout:            timeout,
	})
}
