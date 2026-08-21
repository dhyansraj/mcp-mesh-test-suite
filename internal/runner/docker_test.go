package runner

import "testing"

func TestResolveContainerConfigNetwork(t *testing.T) {
	tests := []struct {
		name   string
		config *ContainerConfig
		want   string
	}{
		{"nil config", nil, "bridge"},
		{"unset network", &ContainerConfig{Image: "python:3.11-slim"}, "bridge"},
		{"blank network", &ContainerConfig{Network: "   "}, "bridge"},
		{"custom network", &ContainerConfig{Network: "my-net"}, "my-net"},
		{"host network", &ContainerConfig{Network: "host"}, "host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveContainerConfig(tt.config).Network; got != tt.want {
				t.Errorf("resolveContainerConfig().Network = %q, want %q", got, tt.want)
			}
		})
	}
}

// buildHostConfig produces the HostConfig handed to ContainerCreate, so this
// asserts the configured network actually reaches the container create call.
func TestBuildHostConfigNetworkMode(t *testing.T) {
	tests := []struct {
		name    string
		network string
		want    string
	}{
		{"configured network", "my-net", "my-net"},
		{"empty network defaults", "", "bridge"},
		{"blank network defaults", "  ", "bridge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &DockerExecutor{config: ContainerConfig{Network: tt.network}}
			hc := e.buildHostConfig(nil)
			if string(hc.NetworkMode) != tt.want {
				t.Errorf("NetworkMode = %q, want %q", hc.NetworkMode, tt.want)
			}
		})
	}
}

func TestBuildHostConfigFromResolvedConfig(t *testing.T) {
	e := &DockerExecutor{config: resolveContainerConfig(&ContainerConfig{Image: "img", Network: "mesh-net"})}
	if got := string(e.buildHostConfig(nil).NetworkMode); got != "mesh-net" {
		t.Errorf("NetworkMode = %q, want %q", got, "mesh-net")
	}
}
