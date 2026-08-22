package worker

import "testing"

func TestNewDockerHandlerNetwork(t *testing.T) {
	tests := []struct {
		name    string
		network string
		want    string
	}{
		{"configured network", "my-net", "my-net"},
		{"unset network defaults to bridge", "", "bridge"},
		{"blank network defaults to bridge", "   ", "bridge"},
		{"explicit bridge", "bridge", "bridge"},
		{"host network", "host", "host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewDockerHandler("http://api", "/suite", "/work", "img:latest", tt.network, "run-1")
			if h.network != tt.want {
				t.Errorf("handler.network = %q, want %q", h.network, tt.want)
			}
			// The same value must land on the config passed to the executor.
			if got := h.containerConfig().Network; got != tt.want {
				t.Errorf("containerConfig().Network = %q, want %q", got, tt.want)
			}
		})
	}
}
