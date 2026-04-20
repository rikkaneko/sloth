package container

import (
	"reflect"
	"testing"
)

func TestCommandEngineBuildInvocationWithoutSudo(t *testing.T) {
	engine := NewCommandEngine("docker", RuntimeOptions{})

	binary, args := engine.buildInvocation([]string{"exec", "-i", "svc"})
	if binary != "docker" {
		t.Fatalf("expected docker binary, got %s", binary)
	}

	expectedArgs := []string{"exec", "-i", "svc"}
	if !reflect.DeepEqual(args, expectedArgs) {
		t.Fatalf("unexpected args: %+v", args)
	}
}

func TestCommandEngineBuildInvocationWithSudo(t *testing.T) {
	engine := NewCommandEngine("podman", RuntimeOptions{
		UseSudo:     true,
		SudoProgram: "doas",
	})

	binary, args := engine.buildInvocation([]string{"cp", "a", "b"})
	if binary != "doas" {
		t.Fatalf("expected doas binary, got %s", binary)
	}

	expectedArgs := []string{"podman", "cp", "a", "b"}
	if !reflect.DeepEqual(args, expectedArgs) {
		t.Fatalf("unexpected args: %+v", args)
	}
}

func TestCommandEngineRuntimeCommandWithSudoDefaultProgram(t *testing.T) {
	engine := NewCommandEngine("docker", RuntimeOptions{
		UseSudo: true,
	})

	command := engine.RuntimeCommand()
	if command != "'sudo' 'docker'" {
		t.Fatalf("expected sudo runtime command, got %s", command)
	}
}
