package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/geanlabs/gean/internal/logger"
)

// proverMemoryFloorBytes is the working-set budget a proving node needs: the
// resident prover arena plus one block proof's transient peak. Matches the
// README "Memory requirements" section. An OOM kill is a silent SIGKILL, so
// the only useful diagnostic is the one emitted before it happens.
const proverMemoryFloorBytes = 12 << 30

// cgroup v1 reports "no limit" as a page-rounded MaxInt64; anything this
// large is unlimited in practice.
const cgroupNoLimitThreshold = 1 << 48

func warnIfMemoryLimited(willProve bool) {
	if !willProve {
		return
	}
	limit, ok := containerMemoryLimitBytes()
	if !ok || limit >= proverMemoryFloorBytes {
		return
	}
	logger.Warn(logger.Node,
		"container memory limit %.1fGiB is below the ~%dGiB proving budget; proposal/aggregation proving risks an OOM kill (silent SIGKILL — check the container runtime's OOMKilled flag, not this log)",
		float64(limit)/(1<<30), proverMemoryFloorBytes>>30)
}

func containerMemoryLimitBytes() (uint64, bool) {
	for _, path := range []string{
		"/sys/fs/cgroup/memory.max",
		"/sys/fs/cgroup/memory/memory.limit_in_bytes",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if limit, ok := parseCgroupLimit(string(data)); ok {
			return limit, true
		}
	}
	return 0, false
}

func parseCgroupLimit(raw string) (uint64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" || s == "max" {
		return 0, false
	}
	limit, err := strconv.ParseUint(s, 10, 64)
	if err != nil || limit >= cgroupNoLimitThreshold {
		return 0, false
	}
	return limit, true
}
