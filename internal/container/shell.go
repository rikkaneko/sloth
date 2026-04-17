package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

func RunHostShell(ctx context.Context, command string, env map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	mergedEnv := os.Environ()
	for key, value := range env {
		mergedEnv = append(mergedEnv, fmt.Sprintf("%s=%s", key, value))
	}
	cmd.Env = mergedEnv

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run host shell: %w", err)
	}

	return nil
}
