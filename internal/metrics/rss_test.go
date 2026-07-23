package metrics

import "testing"

func TestParseVmRSSBytes(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   uint64
	}{
		{"typical", "VmPeak:\t 1000 kB\nVmRSS:\t  204800 kB\nVmData:\t 500 kB\n", 204800 * 1024},
		{"missing", "VmPeak:\t 1000 kB\nVmData:\t 500 kB\n", 0},
		{"malformed", "VmRSS:\tnot-a-number kB\n", 0},
		{"empty value", "VmRSS:\n", 0},
		{"empty input", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseVmRSSBytes([]byte(tc.status)); got != tc.want {
				t.Fatalf("parseVmRSSBytes=%d, want %d", got, tc.want)
			}
		})
	}
}
