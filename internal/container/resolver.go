package container

import (
	"context"
	"fmt"
)

func ResolveEngine(ctx context.Context, explicit string, configured string, containerName string) (Engine, error) {
	chosen := explicit
	if chosen == "" {
		chosen = configured
	}

	if chosen != "" {
		engine, err := NewEngine(chosen)
		if err != nil {
			return nil, err
		}
		return engine, nil
	}

	if containerName == "" {
		return nil, fmt.Errorf("container_name is required for engine auto-detection")
	}

	for _, candidate := range []string{"podman", "docker"} {
		engine, err := NewEngine(candidate)
		if err != nil {
			return nil, err
		}
		exists, err := engine.ContainerExists(ctx, containerName)
		if err != nil {
			return nil, err
		}
		if exists {
			return engine, nil
		}
	}

	return nil, fmt.Errorf("unable to detect engine: container %q not found in podman or docker", containerName)
}
