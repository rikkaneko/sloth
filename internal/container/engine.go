package container

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

type Engine interface {
	Name() string
	ContainerExists(ctx context.Context, containerName string) (bool, error)
	Exec(ctx context.Context, containerName string, command string, env map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error
	CopyFrom(ctx context.Context, containerName string, sourcePath string, destPath string) error
	CopyTo(ctx context.Context, containerName string, sourcePath string, destPath string) error
}

func NewEngine(name string) (Engine, error) {
	switch name {
	case "docker":
		return NewCommandEngine("docker"), nil
	case "podman":
		return NewCommandEngine("podman"), nil
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
