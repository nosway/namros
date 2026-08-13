package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nosway/namros/internal/auth/credentials"
	"github.com/nosway/namros/internal/auth/sigv4"
)

func TestSignRequestVerifiesWithGatewaySigV4Verifier(t *testing.T) {
	fixed := time.Date(2026, 7, 30, 12, 34, 56, 0, time.UTC)
	req, err := http.NewRequest(http.MethodPut, "http://example.test/bucket/key?partNumber=1&uploadId=abc", bytes.NewReader([]byte("payload")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = req.URL.Host

	if err := signRequest(req, signer{
		accessKeyID:     "bench-access",
		secretAccessKey: "bench-secret",
		region:          "us-east-1",
		now:             func() time.Time { return fixed },
	}); err != nil {
		t.Fatalf("signRequest: %v", err)
	}

	store, err := credentials.NewStaticStore(credentials.Credential{
		AccessKeyID:     "bench-access",
		SecretAccessKey: "bench-secret",
		Active:          true,
	})
	if err != nil {
		t.Fatalf("NewStaticStore: %v", err)
	}
	verifier := sigv4.NewVerifier(sigv4.Config{
		Region:      "us-east-1",
		Credentials: store,
		Now:         func() time.Time { return fixed },
	})
	if _, err := verifier.Verify(context.Background(), req); err != nil {
		t.Fatalf("Verify signed request: %v", err)
	}
}

func TestCanonicalQueryStringUsesAWSPercentEncoding(t *testing.T) {
	u := &url.URL{Scheme: "http", Host: "example.test", Path: "/bucket"}
	values := u.Query()
	values.Set("prefix", "bench/small/")
	values.Set("list-type", "2")
	u.RawQuery = values.Encode()

	got := canonicalQueryString(u)
	want := "list-type=2&prefix=bench%2Fsmall%2F"
	if got != want {
		t.Fatalf("canonicalQueryString() = %q want %q", got, want)
	}
}

func TestMakePayloadIsDeterministic(t *testing.T) {
	a := makePayload(1024, 7)
	b := makePayload(1024, 7)
	c := makePayload(1024, 8)
	if !bytes.Equal(a, b) {
		t.Fatal("same seed produced different payload")
	}
	if bytes.Equal(a, c) {
		t.Fatal("different seeds produced identical payload")
	}
}

func TestPutObjectAppliesStorageClassHeader(t *testing.T) {
	runner := &benchRunner{cfg: benchConfig{StorageClass: "EC_4_2"}}
	if got := runner.putObjectHeaders().Get("X-Amz-Storage-Class"); got != "EC_4_2" {
		t.Fatalf("X-Amz-Storage-Class = %q, want EC_4_2", got)
	}
	runner.cfg.StorageClass = " "
	if got := runner.putObjectHeaders().Get("X-Amz-Storage-Class"); got != "" {
		t.Fatalf("blank storage class header = %q, want empty", got)
	}
}

func TestParseSizeSuffixes(t *testing.T) {
	tests := map[string]int{
		"4096": 4096,
		"4K":   4 << 10,
		"4KiB": 4 << 10,
		"2M":   2 << 20,
	}
	for input, want := range tests {
		got, err := parseSize(input)
		if err != nil {
			t.Fatalf("parseSize(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("parseSize(%q) = %d want %d", input, got, want)
		}
	}
}

func TestMemorySamplerWritesJSONLAndSummary(t *testing.T) {
	started := time.Now().UTC().Add(-time.Second)
	memoryPath := filepath.Join(t.TempDir(), "memory.jsonl")
	runner := &benchRunner{cfg: benchConfig{
		MemoryJSONL:    memoryPath,
		MemoryInterval: time.Hour,
	}}
	sampler, err := runner.startMemorySampler(started)
	if err != nil {
		t.Fatalf("startMemorySampler() error = %v", err)
	}
	runner.record(benchResult{
		Status:        "ok",
		RequestBytes:  10,
		ResponseBytes: 20,
	})
	runner.record(benchResult{
		Status: "error",
		Error:  "timeout",
	})
	if err := sampler.Close(); err != nil {
		t.Fatalf("memorySampler.Close() error = %v", err)
	}
	payload, err := os.ReadFile(memoryPath)
	if err != nil {
		t.Fatalf("ReadFile(memory jsonl) error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(payload)), "\n")
	if len(lines) < 2 {
		t.Fatalf("memory sample lines = %d, want at least start and stop", len(lines))
	}
	var last memorySample
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatalf("memory sample JSON decode: %v", err)
	}
	if last.Schema != "namros.s3bench.memory_sample.v1" {
		t.Fatalf("memory sample schema = %q", last.Schema)
	}
	if last.RecordedOperations != 2 || last.ErrorCount != 1 || last.RequestBytes != 10 || last.ResponseBytes != 20 {
		t.Fatalf("memory sample counters = ops %d errors %d req %d resp %d", last.RecordedOperations, last.ErrorCount, last.RequestBytes, last.ResponseBytes)
	}
	summary := runner.summary(started, started.Add(time.Second))
	if summary.MemoryJSONL != memoryPath {
		t.Fatalf("summary memory_jsonl = %q, want %q", summary.MemoryJSONL, memoryPath)
	}
	if summary.Memory == nil {
		t.Fatal("summary memory is nil")
	}
	if summary.Memory.Samples < 2 {
		t.Fatalf("summary memory samples = %d, want at least 2", summary.Memory.Samples)
	}
	if summary.Memory.PeakHeapAllocBytes == 0 || summary.Memory.PeakGoroutines == 0 {
		t.Fatalf("summary memory peaks = %+v", summary.Memory)
	}
}

func TestMetadataListIndexBenchReportsPagesAndGates(t *testing.T) {
	dir := t.TempDir()
	summaryPath := filepath.Join(dir, "summary.json")
	pagePath := filepath.Join(dir, "pages.jsonl")
	summary, err := executeMetadataListIndexBench(context.Background(), metadataListBenchConfig{
		MetadataBackend: "pebble",
		MetadataPath:    filepath.Join(dir, "meta"),
		TenantID:        "tenant-1",
		Bucket:          "list-bench",
		KeyPrefix:       "bench/list",
		ObjectCount:     25,
		PageSize:        10,
		SummaryJSON:     summaryPath,
		PageJSONL:       pagePath,
		FailOnGate:      true,
	})
	if err != nil {
		t.Fatalf("executeMetadataListIndexBench() error = %v", err)
	}
	if summary.SchemaVersion != "namros.s3bench.metadata_list_index.v1" || summary.Status != "passed" {
		t.Fatalf("summary status = %+v", summary)
	}
	if summary.PageCount != 3 || summary.ListedObjects != 25 || summary.MaxPageReadEstimate > summary.PageSize+3 {
		t.Fatalf("summary pages/listed/read estimate = %+v", summary)
	}
	if len(summary.Gates) != 3 {
		t.Fatalf("gates = %+v", summary.Gates)
	}
	for _, gate := range summary.Gates {
		if gate.Status != "passed" {
			t.Fatalf("gate = %+v", gate)
		}
	}

	if err := writeMetadataListBenchSummary(summary, summaryPath); err != nil {
		t.Fatalf("writeMetadataListBenchSummary() error = %v", err)
	}
	payload, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("ReadFile(summary) error = %v", err)
	}
	var decoded metadataListBenchSummary
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("summary JSON decode: %v body=%s", err, string(payload))
	}
	if decoded.PageCount != summary.PageCount || decoded.ListedObjects != summary.ListedObjects {
		t.Fatalf("decoded summary = %+v", decoded)
	}

	pagePayload, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatalf("ReadFile(page jsonl) error = %v", err)
	}
	pageLines := strings.Split(strings.TrimSpace(string(pagePayload)), "\n")
	if len(pageLines) != 3 {
		t.Fatalf("page jsonl lines = %d, want 3: %s", len(pageLines), string(pagePayload))
	}
	var lastPage metadataListBenchPage
	if err := json.Unmarshal([]byte(pageLines[len(pageLines)-1]), &lastPage); err != nil {
		t.Fatalf("page JSON decode: %v", err)
	}
	if lastPage.IsTruncated || lastPage.Objects != 5 {
		t.Fatalf("last page = %+v", lastPage)
	}
}

func TestNoisyTenantProfileReportsNeighborProgress(t *testing.T) {
	summary, err := executeNoisyTenantProfile(noisyTenantProfileConfig{
		NoisyTenantID:          "tenant-noisy",
		NeighborTenantID:       "tenant-neighbor",
		MaxConcurrentGlobal:    2,
		MaxConcurrentPerTenant: 1,
		NoisyHoldRequests:      1,
		NoisyAttempts:          4,
		NeighborAttempts:       3,
		FailOnGate:             true,
	})
	if err != nil {
		t.Fatalf("executeNoisyTenantProfile() error = %v", err)
	}
	if summary.SchemaVersion != "namros.s3bench.noisy_tenant_profile.v1" || summary.Status != "passed" {
		t.Fatalf("summary = %+v", summary)
	}
	noisy := noisyTenantStatsByRole(t, summary, "noisy")
	neighbor := noisyTenantStatsByRole(t, summary, "neighbor")
	if noisy.Throttled == 0 {
		t.Fatalf("noisy tenant throttled = 0, summary = %+v", summary)
	}
	if neighbor.Completed != 3 || neighbor.Throttled != 0 {
		t.Fatalf("neighbor stats = %+v, want completed=3 throttled=0", neighbor)
	}
	if !noisyTenantGatePassed(summary, "neighbor_admitted_while_noisy_throttled") {
		t.Fatalf("neighbor progress gate not passed: %+v", summary.Gates)
	}
	if len(summary.Events) == 0 {
		t.Fatal("summary events are empty")
	}
}

func TestNoisyTenantProfileFailsWhenGlobalCapacityStarvesNeighbor(t *testing.T) {
	summary, err := executeNoisyTenantProfile(noisyTenantProfileConfig{
		NoisyTenantID:          "tenant-noisy",
		NeighborTenantID:       "tenant-neighbor",
		MaxConcurrentGlobal:    1,
		MaxConcurrentPerTenant: 1,
		NoisyHoldRequests:      1,
		NoisyAttempts:          2,
		NeighborAttempts:       1,
		FailOnGate:             false,
	})
	if err != nil {
		t.Fatalf("executeNoisyTenantProfile() error = %v", err)
	}
	if summary.Status != "failed" {
		t.Fatalf("summary status = %q, want failed: %+v", summary.Status, summary)
	}
	neighbor := noisyTenantStatsByRole(t, summary, "neighbor")
	if neighbor.Completed != 0 || neighbor.Throttled != 1 {
		t.Fatalf("neighbor stats = %+v, want completed=0 throttled=1", neighbor)
	}
	if noisyTenantGatePassed(summary, "neighbor_completed_without_throttle") {
		t.Fatalf("neighbor completion gate unexpectedly passed: %+v", summary.Gates)
	}
}

func TestNoisyTenantProfileWritesSummaryAndEvents(t *testing.T) {
	dir := t.TempDir()
	summaryPath := filepath.Join(dir, "summary.json")
	eventPath := filepath.Join(dir, "events.jsonl")
	summary, err := executeNoisyTenantProfile(noisyTenantProfileConfig{
		NoisyTenantID:          "tenant-noisy",
		NeighborTenantID:       "tenant-neighbor",
		MaxConcurrentGlobal:    2,
		MaxConcurrentPerTenant: 1,
		NoisyHoldRequests:      1,
		NoisyAttempts:          2,
		NeighborAttempts:       2,
		EventJSONL:             eventPath,
		FailOnGate:             true,
	})
	if err != nil {
		t.Fatalf("executeNoisyTenantProfile() error = %v", err)
	}
	if err := writeNoisyTenantProfileSummary(summary, summaryPath); err != nil {
		t.Fatalf("writeNoisyTenantProfileSummary() error = %v", err)
	}
	if err := writeNoisyTenantEvents(summary.Events, eventPath); err != nil {
		t.Fatalf("writeNoisyTenantEvents() error = %v", err)
	}
	payload, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("ReadFile(summary) error = %v", err)
	}
	var decoded noisyTenantProfileSummary
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("summary JSON decode: %v body=%s", err, string(payload))
	}
	if decoded.Status != "passed" || decoded.EventJSONL != eventPath {
		t.Fatalf("decoded summary = %+v", decoded)
	}
	eventPayload, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatalf("ReadFile(events) error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(eventPayload)), "\n")
	if len(lines) != len(summary.Events) {
		t.Fatalf("event lines = %d, want %d: %s", len(lines), len(summary.Events), string(eventPayload))
	}
	var first noisyTenantEvent
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("event JSON decode: %v", err)
	}
	if first.Schema != "namros.s3bench.noisy_tenant_event.v1" || first.Sequence != 1 {
		t.Fatalf("first event = %+v", first)
	}
}

func noisyTenantStatsByRole(t *testing.T, summary noisyTenantProfileSummary, role string) noisyTenantStats {
	t.Helper()
	for _, tenant := range summary.Tenants {
		if tenant.Role == role {
			return tenant
		}
	}
	t.Fatalf("missing tenant role %q in %+v", role, summary.Tenants)
	return noisyTenantStats{}
}

func noisyTenantGatePassed(summary noisyTenantProfileSummary, name string) bool {
	for _, gate := range summary.Gates {
		if gate.Name == name {
			return gate.Status == "passed"
		}
	}
	return false
}
