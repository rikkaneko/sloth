package container

import (
	"context"
	"fmt"
	"strings"
)

type Resolution struct {
	Engine        Engine
	ContainerName string
}

var engineFactory = NewEngine

func ResolveEngine(
	ctx context.Context,
	explicitEngine string,
	configuredEngine string,
	explicitContainerName string,
	configuredContainerName string,
	serviceID string,
	forceLocal bool,
) (Resolution, error) {
	if forceLocal {
		engine, err := engineFactory("local")
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{Engine: engine}, nil
	}

	chosen := strings.TrimSpace(explicitEngine)
	if chosen == "local" {
		return Resolution{}, fmt.Errorf("use --local instead of --engine local")
	}
	if chosen == "" {
		chosen = strings.TrimSpace(configuredEngine)
	}
	containerName := chooseContainerName(explicitContainerName, configuredContainerName, serviceID)

	if chosen != "" {
		engine, err := engineFactory(chosen)
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{
			Engine:        engine,
			ContainerName: containerName,
		}, nil
	}

	if containerName == "" {
		return Resolution{}, fmt.Errorf("container_name is required for engine auto-detection")
	}

	for _, candidate := range []string{"podman", "docker"} {
		engine, err := engineFactory(candidate)
		if err != nil {
			return Resolution{}, err
		}
		exists, err := engine.ContainerExists(ctx, containerName)
		if err != nil {
			return Resolution{}, err
		}
		if exists {
			return Resolution{
				Engine:        engine,
				ContainerName: containerName,
			}, nil
		}
	}

	return Resolution{}, fmt.Errorf("unable to detect engine: container %q not found in podman or docker", containerName)
}

func chooseContainerName(explicitContainerName string, configuredContainerName string, serviceID string) string {
	candidates := []string{explicitContainerName, configuredContainerName, serviceID}
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
