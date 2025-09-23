package docker

import (
	"context"
	"sort"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// ContainerNetInfo is a minimal view used for network grouping.
type ContainerNetInfo struct {
	ID       string
	Name     string
	Status   string
	Networks []string
}

// CollectNetworks groups containers by the networks they are connected to.
// Containers with no networks are placed under the "(none)" group.
func CollectNetworks(ctx context.Context, cli *client.Client, includeAll bool) (map[string][]ContainerNetInfo, error) {
	listOpts := container.ListOptions{All: includeAll}
	containers, err := cli.ContainerList(ctx, listOpts)
	if err != nil {
		return nil, err
	}
	groups := make(map[string][]ContainerNetInfo)
	for _, c := range containers {
		info := ContainerNetInfo{
			ID:     c.ID,
			Name:   deriveName(c.Names),
			Status: deriveStatus(c.State, c.Status),
		}
		nets := extractNetworkNames(c.NetworkSettings)
		if len(nets) == 0 {
			nets = []string{"(none)"}
		}
		info.Networks = nets
		for _, n := range nets {
			groups[n] = append(groups[n], info)
		}
	}
	// Sort each group's containers by name asc for stable output
	for n := range groups {
		sort.Slice(groups[n], func(i, j int) bool {
			return strings.ToLower(groups[n][i].Name) < strings.ToLower(groups[n][j].Name)
		})
	}
	return groups, nil
}

func extractNetworkNames(ns *types.SummaryNetworkSettings) []string {
	if ns == nil || ns.Networks == nil {
		return nil
	}
	names := make([]string, 0, len(ns.Networks))
	for name := range ns.Networks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
