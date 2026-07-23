package metrics

import (
	"bytes"
	"os"
	"strconv"
)

// SampleProcessRSS reads the process resident set size and updates the
// lean_node_rss_bytes gauge. The prover arena lives outside the Go heap, so
// Go runtime metrics alone understate real memory pressure; RSS is what the
// kernel OOM killer judges. No-op where /proc is unavailable.
func SampleProcessRSS() {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return
	}
	if rss := parseVmRSSBytes(data); rss > 0 {
		metricProcessRSSBytes.Set(float64(rss))
	}
}

func parseVmRSSBytes(status []byte) uint64 {
	for _, line := range bytes.Split(status, []byte("\n")) {
		rest, ok := bytes.CutPrefix(line, []byte("VmRSS:"))
		if !ok {
			continue
		}
		fields := bytes.Fields(rest)
		if len(fields) < 1 {
			return 0
		}
		kb, err := strconv.ParseUint(string(fields[0]), 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
}
