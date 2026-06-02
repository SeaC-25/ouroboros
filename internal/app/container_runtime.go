package app

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const containerRuntimeCheckTimeout = 5 * time.Second

func CheckContainerRuntime(config ContainerExecutionConfig) error {
	return checkContainerRuntime(config, containerRuntimeCheckTimeout)
}

func checkContainerRuntime(config ContainerExecutionConfig, timeout time.Duration) error {
	engine := containerEngine(config)
	resolved, err := exec.LookPath(engine)
	if err != nil {
		return fmt.Errorf("container engine %s unavailable: %w", engine, err)
	}

	image := strings.TrimSpace(config.Image)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, resolved, "image", "inspect", image)
	cmd.Env = containerEngineEnvironment()
	output, err := cmd.CombinedOutput()
	message := strings.TrimSpace(string(output))
	if ctx.Err() != nil {
		return fmt.Errorf("container image %s inspect through %s timed out: %w", image, engine, ctx.Err())
	}
	if err != nil {
		if message != "" {
			return fmt.Errorf("container image %s unavailable through %s: %w: %s", image, engine, err, message)
		}
		return fmt.Errorf("container image %s unavailable through %s: %w", image, engine, err)
	}
	return nil
}
