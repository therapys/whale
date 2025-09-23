package docker

import (
	"context"

	"github.com/docker/docker/client"
)

// NewClient creates a Docker API client using environment variables and
// negotiates the API version with the daemon for compatibility.
func NewClient(ctx context.Context) (*client.Client, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}
	// Ping to validate connectivity early; not strictly required but provides
	// a fast-fail if the daemon is unreachable.
	if _, err := cli.Ping(ctx); err != nil {
		// Return the client alongside the error would be odd for callers.
		// Close the client to avoid leaking resources.
		_ = cli.Close()
		return nil, err
	}
	return cli, nil
}
