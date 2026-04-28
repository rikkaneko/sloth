package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"sloth/internal/ui"
)

type CommandEngine struct {
	binary  string
	runtime RuntimeOptions
}

func NewCommandEngine(binary string, runtime RuntimeOptions) CommandEngine {
	return CommandEngine{binary: binary, runtime: runtime}
}

func (e CommandEngine) Name() string {
	return e.binary
}

func (e CommandEngine) RuntimeCommand() string {
	if !e.runtime.UseSudo {
		return shellEscapeToken(e.binary)
	}
	return shellEscapeToken(e.runtime.NormalizeSudoProgram()) + " " + shellEscapeToken(e.binary)
}

func (e CommandEngine) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	if containerName == "" {
		return false, nil
	}
	if !IsBinaryAvailable(e.binary) {
		return false, nil
	}

	args := []string{"container", "inspect", containerName}
	invocationBinary, invocationArgs := e.buildInvocation(args)
	cmd := exec.CommandContext(ctx, invocationBinary, invocationArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	ui.Debugf("run_cmd %s", renderCommand(invocationBinary, invocationArgs))
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			logCommandOutput("", stderr.String())
			return false, nil
		}
		logCommandOutput("", stderr.String())
		return false, fmt.Errorf("inspect container with %s: %w", e.binary, err)
	}
	logCommandOutput("", stderr.String())

	return true, nil
}

func (e CommandEngine) Exec(ctx context.Context, containerName string, command string, env map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	args := []string{"exec", "-i"}
	for key, value := range env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
	}
	args = append(args, containerName, "sh", "-c", command)

	invocationBinary, invocationArgs := e.buildInvocation(args)
	cmd := exec.CommandContext(ctx, invocationBinary, invocationArgs...)
	cmd.Stdin = stdin
	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer
	cmd.Stdout = writerWithCapture(stdout, &stdoutBuffer)
	cmd.Stderr = writerWithCapture(stderr, &stderrBuffer)
	ui.Debugf("run_cmd %s", renderCommand(invocationBinary, invocationArgs))

	if err := cmd.Run(); err != nil {
		logCommandOutput(stdoutBuffer.String(), stderrBuffer.String())
		stderrMessage := strings.TrimSpace(stderrBuffer.String())
		if stderrMessage != "" {
			return fmt.Errorf("execute command in %s container %s: %w (%s)", e.binary, containerName, err, stderrMessage)
		}
		return fmt.Errorf("execute command in %s container %s: %w", e.binary, containerName, err)
	}
	logCommandOutput(stdoutBuffer.String(), stderrBuffer.String())

	return nil
}

func (e CommandEngine) CopyFrom(ctx context.Context, containerName string, sourcePath string, destPath string) error {
	args := []string{"cp", fmt.Sprintf("%s:%s", containerName, sourcePath), destPath}
	invocationBinary, invocationArgs := e.buildInvocation(args)
	cmd := exec.CommandContext(ctx, invocationBinary, invocationArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	ui.Debugf("run_cmd %s", renderCommand(invocationBinary, invocationArgs))

	if err := cmd.Run(); err != nil {
		logCommandOutput("", stderr.String())
		return fmt.Errorf("copy from container failed: %w (%s)", err, stderr.String())
	}
	logCommandOutput("", stderr.String())
	return nil
}

func (e CommandEngine) CopyTo(ctx context.Context, containerName string, sourcePath string, destPath string) error {
	args := []string{"cp", sourcePath, fmt.Sprintf("%s:%s", containerName, destPath)}
	invocationBinary, invocationArgs := e.buildInvocation(args)
	cmd := exec.CommandContext(ctx, invocationBinary, invocationArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	ui.Debugf("run_cmd %s", renderCommand(invocationBinary, invocationArgs))

	if err := cmd.Run(); err != nil {
		logCommandOutput("", stderr.String())
		return fmt.Errorf("copy to container failed: %w (%s)", err, stderr.String())
	}
	logCommandOutput("", stderr.String())
	return nil
}

func (e CommandEngine) RunHostShell(ctx context.Context, command string, env map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdin = stdin
	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer
	cmd.Stdout = writerWithCapture(stdout, &stdoutBuffer)
	cmd.Stderr = writerWithCapture(stderr, &stderrBuffer)

	mergedEnv := os.Environ()
	for key, value := range env {
		mergedEnv = append(mergedEnv, fmt.Sprintf("%s=%s", key, value))
	}
	cmd.Env = mergedEnv
	ui.Debugf("run_cmd sh -c %q", command)

	if err := cmd.Run(); err != nil {
		logCommandOutput(stdoutBuffer.String(), stderrBuffer.String())
		stderrMessage := strings.TrimSpace(stderrBuffer.String())
		if stderrMessage != "" {
			return fmt.Errorf("run host shell for %s: %w (%s)", e.binary, err, stderrMessage)
		}
		return fmt.Errorf("run host shell for %s: %w", e.binary, err)
	}
	logCommandOutput(stdoutBuffer.String(), stderrBuffer.String())

	return nil
}

func asExitError(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		*target = exitErr
	}
	return ok
}

func writerWithCapture(writer io.Writer, buffer *bytes.Buffer) io.Writer {
	if writer == nil {
		return buffer
	}
	return io.MultiWriter(writer, buffer)
}

func logCommandOutput(stdout string, stderr string) {
	stdoutText := strings.TrimSpace(stdout)
	stderrText := strings.TrimSpace(stderr)
	if stdoutText != "" {
		ui.Debugf("Command stdout:\n%s", stdoutText)
	}
	if stderrText != "" {
		ui.Debugf("Command stderr:\n%s", stderrText)
	}
}

func renderCommand(binary string, args []string) string {
	return strings.TrimSpace(binary + " " + strings.Join(args, " "))
}

func (e CommandEngine) buildInvocation(args []string) (string, []string) {
	if !e.runtime.UseSudo {
		return e.binary, args
	}
	combined := make([]string, 0, len(args)+1)
	combined = append(combined, e.binary)
	combined = append(combined, args...)
	return e.runtime.NormalizeSudoProgram(), combined
}

func shellEscapeToken(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
