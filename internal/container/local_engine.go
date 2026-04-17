package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type LocalEngine struct{}

func (LocalEngine) Name() string {
	return "local"
}

func (LocalEngine) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	return false, nil
}

func (LocalEngine) Exec(ctx context.Context, containerName string, command string, env map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
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
		return fmt.Errorf("execute local command: %w", err)
	}

	return nil
}

func (LocalEngine) CopyFrom(ctx context.Context, containerName string, sourcePath string, destPath string) error {
	return fmt.Errorf("local engine does not support CopyFrom")
}

func (LocalEngine) CopyTo(ctx context.Context, containerName string, sourcePath string, destPath string) error {
	return fmt.Errorf("local engine does not support CopyTo")
}
