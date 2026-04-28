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

func RunHostShell(ctx context.Context, command string, env map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
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
			return fmt.Errorf("run host shell: %w (%s)", err, stderrMessage)
		}
		return fmt.Errorf("run host shell: %w", err)
	}
	logCommandOutput(stdoutBuffer.String(), stderrBuffer.String())

	return nil
}
