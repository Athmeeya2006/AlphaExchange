package main

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// launchSandbox creates and starts a hardened contestant container.
// Returns the container IP on the shared Docker network and the internal port (8080).
// Build-worker and bot-fleet reach the contestant via container IP on the same Docker network.
func launchSandbox(ctx context.Context, cli *client.Client, cfg Config, imageName, submissionID string) (containerID, ip string, port int, err error) {
	exposed := nat.PortSet{"8080/tcp": struct{}{}}

	pidsLimit := int64(50)
	mem := cfg.SandboxMemoryMB * 1024 * 1024

	containerCfg := &container.Config{
		Image:        imageName,
		ExposedPorts: exposed,
		Labels: map[string]string{
			"trade-eval":    "contestant",
			"submission-id": submissionID,
		},
	}

	// seccomp: Docker SecurityOpt requires the JSON content inline, not a file path.
	secOpt := []string{"no-new-privileges:true"}
	if cfg.SeccompProfile != "" {
		data, readErr := os.ReadFile(cfg.SeccompProfile)
		if readErr == nil {
			secOpt = append(secOpt, "seccomp="+string(data))
		}
		// If we cannot read the file, skip seccomp rather than failing the build.
	}

	hostCfg := &container.HostConfig{
		Resources: container.Resources{
			Memory:     mem,
			MemorySwap: mem,
			CPUQuota:   100000,
			CpusetCpus: cfg.SandboxCPUCores,
			PidsLimit:  &pidsLimit,
		},
		NetworkMode:    container.NetworkMode(cfg.ContestantNetwork),
		ReadonlyRootfs: true,
		Tmpfs:          map[string]string{"/tmp": "rw,noexec,nosuid,size=64m"},
		SecurityOpt:    secOpt,
		CapDrop:        []string{"ALL"},
		CapAdd:         []string{},
		AutoRemove:     false,
		RestartPolicy:  container.RestartPolicy{Name: "no"},
	}

	netCfg := &network.NetworkingConfig{}

	created, err := cli.ContainerCreate(ctx, containerCfg, hostCfg, netCfg, nil, "contestant-"+submissionID)
	if err != nil {
		return "", "", 0, fmt.Errorf("container create: %w", err)
	}
	containerID = created.ID

	if err := cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return containerID, "", 0, fmt.Errorf("container start: %w", err)
	}

	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return containerID, "", 0, fmt.Errorf("container inspect: %w", err)
	}

	// Resolve the container IP on the shared Docker network.
	// Build-worker and bot-fleet are on the same network so they reach the
	// contestant via the container IP on port 8080 (the internal listening port).
	if inspect.NetworkSettings != nil {
		if netInfo, ok := inspect.NetworkSettings.Networks[cfg.ContestantNetwork]; ok {
			ip = netInfo.IPAddress
		}
		if ip == "" {
			ip = inspect.NetworkSettings.IPAddress
		}
	}

	port = 8080 // containers always listen on 8080; callers on the same network use this directly

	return containerID, ip, port, nil
}
