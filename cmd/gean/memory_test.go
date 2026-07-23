package main

import "testing"

func TestParseCgroupLimit(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want uint64
		ok   bool
	}{
		{"v2 limited", "8589934592\n", 8589934592, true},
		{"v2 unlimited", "max\n", 0, false},
		{"v1 no-limit sentinel", "9223372036854771712\n", 0, false},
		{"garbage", "banana\n", 0, false},
		{"empty", "", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseCgroupLimit(tc.raw)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("parseCgroupLimit(%q)=(%d,%v), want (%d,%v)", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}
