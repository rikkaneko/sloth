package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type CommandEngine struct {
	binary string
}

func NewCommandEngine(binary string) CommandEngine {
	return CommandEngine{binary: binary}
}

func (e CommandEngine) Name() string {
	return e.binary
}

func (e CommandEngine) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	if containerName == "" {
		return false, nil
	}
	if !IsBinaryAvailable(e.binary) {
		return false, nil
	}

	cmd := exec.CommandContext(ctx, e.binary, "container", "inspect", containerName)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			return false, nil
		}
		return false, fmt.Errorf("inspect container with %s: %w", e.binary, err)
	}

	return true, nil
}

func (e CommandEngine) Exec(ctx context.Context, containerName string, command string, env map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	args := []string{"exec", "-i"}
	for key, value := range env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
	}
	args = append(args, containerName, "sh", "-lc", command)

	cmd := exec.CommandContext(ctx, e.binary, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("execute command in %s container %s: %w", e.binary, containerName, err)
	}

	return nil
}

func (e CommandEngine) CopyFrom(ctx context.Context, containerName string, sourcePath string, destPath string) error {
	cmd := exec.CommandContext(ctx, e.binary, "cp", fmt.Sprintf("%s:%s", containerName, sourcePath), destPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy from container failed: %w (%s)", err, stderr.String())
	}
	return nil
}

func (e CommandEngine) CopyTo(ctx context.Context, containerName string, sourcePath string, destPath string) error {
	cmd := exec.CommandContext(ctx, e.binary, "cp", sourcePath, fmt.Sprintf("%s:%s", containerName, destPath))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy to container failed: %w (%s)", err, stderr.String())
	}
	return nil
}

func (e CommandEngine) RunHostShell(ctx context.Context, command string, env map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
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
		return fmt.Errorf("run host shell for %s: %w", e.binary, err)
	}

	return nil
}

func asExitError(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		*target = exitErr
	}
	return ok
}
