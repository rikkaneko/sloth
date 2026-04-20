package container

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type Engine interface {
	Name() string
	RuntimeCommand() string
	ContainerExists(ctx context.Context, containerName string) (bool, error)
	Exec(ctx context.Context, containerName string, command string, env map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error
	CopyFrom(ctx context.Context, containerName string, sourcePath string, destPath string) error
	CopyTo(ctx context.Context, containerName string, sourcePath string, destPath string) error
}

type RuntimeOptions struct {
	UseSudo     bool
	SudoProgram string
}

func (o RuntimeOptions) NormalizeSudoProgram() string {
	program := strings.TrimSpace(o.SudoProgram)
	if program == "" {
		return "sudo"
	}
	return program
}

func NewEngine(name string, runtime RuntimeOptions) (Engine, error) {
	switch name {
	case "docker":
		return NewCommandEngine("docker", runtime), nil
	case "podman":
		return NewCommandEngine("podman", runtime), nil
	case "local":
		return LocalEngine{}, nil
	default:
		return nil, fmt.Errorf("unsupported engine %q", name)
	}
}

func IsBinaryAvailable(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}
