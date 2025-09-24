package docker

import (
	"context"
    "net"
    "net/http"
    "time"

	"github.com/docker/docker/client"
)

// NewClient creates a Docker API client using environment variables and
// negotiates the API version with the daemon for compatibility.
func NewClient(ctx context.Context) (*client.Client, error) {
    // Tuned HTTP transport for high parallelism and fast reuse
    transport := &http.Transport{
        Proxy:               http.ProxyFromEnvironment,
        DialContext:         (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
        ForceAttemptHTTP2:   false,
        MaxIdleConns:        200,
        MaxIdleConnsPerHost: 200,
        IdleConnTimeout:     90 * time.Second,
        TLSHandshakeTimeout: 2 * time.Second,
        ExpectContinueTimeout: 1 * time.Second,
        DisableCompression:  true,
    }
    httpClient := &http.Client{Transport: transport, Timeout: 0}

    cli, err := client.NewClientWithOpts(
        client.FromEnv,
        client.WithHTTPClient(httpClient),
        client.WithAPIVersionNegotiation(),
    )
	if err != nil {
		return nil, err
	}
	return cli, nil
}
