package ragclient

import (
	"testing"
	"time"
)

func TestPerRPCDeadline_TableMatchesPlan(t *testing.T) {
	cases := []struct {
		rpc  string
		want time.Duration
	}{
		{"Search", 10 * time.Second},
		{"IngestBatch", 15 * time.Second},
		{"UpdateACL", 30 * time.Second},
		{"DeleteByDocID", 30 * time.Second},
		{"CreateDataset", 30 * time.Second},
		{"DropDataset", 30 * time.Second},
		{"Prune", 5 * time.Minute},
		{"DeleteByOrg", 30 * time.Minute},
		{"Health", 2 * time.Second},
		{"Unknown", 10 * time.Second}, // default fallback
	}
	for _, tc := range cases {
		if got := perRPCDeadline(tc.rpc); got != tc.want {
			t.Fatalf("perRPCDeadline(%q) = %v, want %v", tc.rpc, got, tc.want)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"missing endpoint", Config{SharedSecret: "s"}, true},
		{"missing secret", Config{Endpoint: "e"}, true},
		{"negative retries", Config{Endpoint: "e", SharedSecret: "s", MaxRetries: -1}, true},
		{"ok", Config{Endpoint: "e", SharedSecret: "s"}, false},
	}
	for _, tc := range cases {
		err := tc.cfg.validate()
		if tc.wantErr && err == nil {
			t.Fatalf("%s: want err, got nil", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("%s: unexpected err %v", tc.name, err)
		}
	}
}
