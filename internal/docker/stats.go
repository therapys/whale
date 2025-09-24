package docker

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// ContainerSnapshot is a one-shot snapshot of container runtime metrics.
type ContainerSnapshot struct {
	ID         string
	Name       string
	Status     string
	CPUPercent float64
	MemUsage   uint64 // bytes
	MemLimit   uint64 // bytes
	MemPercent float64
	NetRx      uint64 // bytes
	NetTx      uint64 // bytes
	BlockRead  uint64 // bytes
	BlockWrite uint64 // bytes
	PIDs       int
}

// CollectSnapshots lists containers and collects a single stats sample for each.
// For stopped containers, metrics are zeroed and status reflects their state.
func CollectSnapshots(ctx context.Context, cli *client.Client, includeAll bool) ([]ContainerSnapshot, error) {
	// List containers. We use All=true only if includeAll is set; otherwise only running.
	listOpts := container.ListOptions{All: includeAll}
	containers, err := cli.ContainerList(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	snapshots := make([]ContainerSnapshot, len(containers))
	runningIdx := make([]int, 0, len(containers))
	for i, c := range containers {
		snapshots[i] = ContainerSnapshot{
			ID:     c.ID,
			Name:   deriveName(c.Names),
			Status: deriveStatus(c.State, c.Status),
		}
		if c.State == "running" {
			runningIdx = append(runningIdx, i)
		}
	}

	// Parallelize stats fetch for running containers with a bounded semaphore and per-call timeout.
	if len(runningIdx) == 0 {
		return snapshots, nil
	}
	concurrency := 16
	if len(runningIdx) < concurrency {
		concurrency = len(runningIdx)
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, idx := range runningIdx {
		i := idx
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer cancel()
			if err := populateStats(cctx, cli, &snapshots[i], snapshots[i].ID); err != nil {
				snapshots[i].Status = "ERROR"
			}
		}()
	}
	wg.Wait()
	return snapshots, nil
}

func deriveName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	n := names[0]
	return strings.TrimPrefix(n, "/")
}

func deriveStatus(state, status string) string {
	// Docker container list provides both a brief state (e.g., running) and a
	// human string in Status (e.g., "Up X minutes"). We prefer Status when set.
	if status != "" {
		return status
	}
	if state != "" {
		return state
	}
	return ""
}

func populateStats(ctx context.Context, cli *client.Client, snap *ContainerSnapshot, containerID string) error {
	// Single snapshot: call ContainerStats with streaming=false.
	stats, err := cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return err
	}
	defer stats.Body.Close()

	// Stats JSON structure mirrors types.StatsJSON.
	decoder := json.NewDecoder(io.LimitReader(stats.Body, 10*1024*1024)) // 10 MiB safety
	var sj container.Stats

	// Stats endpoint returns a single JSON doc when stream=false.
	if err := decoder.Decode(&sj); err != nil {
		return err
	}

	// CPU percentage: (cpuDelta / systemDelta) * onlineCPUs * 100
	cpuPercent := computeCPUPercent(&sj)
	memUsage, memLimit, memPercent := computeMemory(&sj)
	netRx, netTx := computeNetwork(&sj)
	blkRead, blkWrite := computeBlockIO(&sj)
	pids := 0
	if sj.PidsStats.Current != 0 {
		pids = int(sj.PidsStats.Current)
	}

	snap.CPUPercent = cpuPercent
	snap.MemUsage = memUsage
	snap.MemLimit = memLimit
	snap.MemPercent = memPercent
	snap.NetRx = netRx
	snap.NetTx = netTx
	snap.BlockRead = blkRead
	snap.BlockWrite = blkWrite
	snap.PIDs = pids
	return nil
}

func computeCPUPercent(s *container.Stats) float64 {
	// Defensive checks: precpu or system cpu usage may be missing/zero.
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage - s.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(s.CPUStats.SystemUsage - s.PreCPUStats.SystemUsage)

	if cpuDelta <= 0 || systemDelta <= 0 {
		return 0
	}
	// number of CPUs: prefer OnlineCPUs when present, otherwise len(percpu)
	var numCPUs float64
	if s.CPUStats.OnlineCPUs > 0 {
		numCPUs = float64(s.CPUStats.OnlineCPUs)
	} else if len(s.CPUStats.CPUUsage.PercpuUsage) > 0 {
		numCPUs = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	} else {
		numCPUs = 1
	}
	return (cpuDelta / systemDelta) * numCPUs * 100.0
}

func computeMemory(s *container.Stats) (usage uint64, limit uint64, percent float64) {
	usage = s.MemoryStats.Usage
	limit = s.MemoryStats.Limit
	if limit == 0 || usage == 0 {
		return usage, limit, 0
	}
	// Per Docker CLI, memory usage may need adjustment for cached; for MVP we
	// use raw usage to avoid negative values on cgroup v2 with missing fields.
	percent = (float64(usage) / float64(limit)) * 100.0
	return
}

func computeNetwork(s *container.Stats) (rx uint64, tx uint64) {
	// s.Networks is a map[string]types.NetworkStats; sum across entries.
	for _, nw := range s.Networks {
		rx += nw.RxBytes
		tx += nw.TxBytes
	}
	return
}

func computeBlockIO(s *container.Stats) (read uint64, write uint64) {
	// Aggregate by operation from BlkioStats.IOServiceBytesRecursive
	for _, e := range s.BlkioStats.IoServiceBytesRecursive {
		op := strings.ToLower(e.Op)
		switch op {
		case "read":
			read += uint64(e.Value)
		case "write":
			write += uint64(e.Value)
		}
	}
	return
}
