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

type LocalEngine struct{}

func (LocalEngine) Name() string {
	return "local"
}

func (LocalEngine) RuntimeCommand() string {
	return "local"
}

func (LocalEngine) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	return false, nil
}

func (LocalEngine) Exec(ctx context.Context, containerName string, command string, env map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
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
	ui.Debugf("run_cmd sh -lc %q", command)

	if err := cmd.Run(); err != nil {
		logCommandOutput(stdoutBuffer.String(), stderrBuffer.String())
		stderrMessage := strings.TrimSpace(stderrBuffer.String())
		if stderrMessage != "" {
			return fmt.Errorf("execute local command: %w (%s)", err, stderrMessage)
		}
		return fmt.Errorf("execute local command: %w", err)
	}
	logCommandOutput(stdoutBuffer.String(), stderrBuffer.String())

	return nil
}

func (LocalEngine) CopyFrom(ctx context.Context, containerName string, sourcePath string, destPath string) error {
	return fmt.Errorf("local engine does not support CopyFrom")
}

func (LocalEngine) CopyTo(ctx context.Context, containerName string, sourcePath string, destPath string) error {
	return fmt.Errorf("local engine does not support CopyTo")
}
