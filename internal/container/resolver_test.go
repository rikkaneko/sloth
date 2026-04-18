package container

import (
	"context"
	"io"
	"testing"
)

type stubEngine struct {
	name    string
	exists  map[string]bool
	checked []string
}

func (s *stubEngine) Name() string {
	return s.name
}

func (s *stubEngine) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	s.checked = append(s.checked, containerName)
	return s.exists[containerName], nil
}

func (s *stubEngine) Exec(ctx context.Context, containerName string, command string, env map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	return nil
}

func (s *stubEngine) CopyFrom(ctx context.Context, containerName string, sourcePath string, destPath string) error {
	return nil
}

func (s *stubEngine) CopyTo(ctx context.Context, containerName string, sourcePath string, destPath string) error {
	return nil
}

func TestResolveEngineAutoDetectUsesServiceIDWhenContainerMissing(t *testing.T) {
	originalFactory := engineFactory
	defer func() {
		engineFactory = originalFactory
	}()

	podman := &stubEngine{name: "podman", exists: map[string]bool{"svc": true}}
	docker := &stubEngine{name: "docker", exists: map[string]bool{}}

	engineFactory = func(name string) (Engine, error) {
		if name == "podman" {
			return podman, nil
		}
		if name == "docker" {
			return docker, nil
		}
		return &stubEngine{name: name, exists: map[string]bool{}}, nil
	}

	resolution, err := ResolveEngine(context.Background(), "", "", "", "", "svc", false)
	if err != nil {
		t.Fatalf("resolve engine: %v", err)
	}

	if resolution.Engine.Name() != "podman" {
		t.Fatalf("expected podman engine, got %s", resolution.Engine.Name())
	}
	if resolution.ContainerName != "svc" {
		t.Fatalf("expected service-id fallback container name, got %q", resolution.ContainerName)
	}
}

func TestResolveEngineRejectsExplicitLocalEngine(t *testing.T) {
	_, err := ResolveEngine(context.Background(), "local", "", "", "", "svc", false)
	if err == nil {
		t.Fatalf("expected error for --engine local")
	}
}

func TestResolveEngineForceLocal(t *testing.T) {
	resolution, err := ResolveEngine(context.Background(), "", "", "", "", "svc", true)
	if err != nil {
		t.Fatalf("resolve local engine: %v", err)
	}
	if resolution.Engine.Name() != "local" {
		t.Fatalf("expected local engine, got %s", resolution.Engine.Name())
	}
	if resolution.ContainerName != "" {
		t.Fatalf("expected empty container name for local mode")
	}
}
