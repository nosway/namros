package id

import (
	"bytes"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestProcessGeneratorIDsAreLexicallyTimeOrdered(t *testing.T) {
	times := []time.Time{
		time.Unix(100, 1).UTC(),
		time.Unix(100, 2).UTC(),
		time.Unix(100, 3).UTC(),
	}
	index := 0
	generator, err := NewProcessGenerator(
		WithClock(func() time.Time {
			out := times[index]
			if index < len(times)-1 {
				index++
			}
			return out
		}),
		WithGeneratorID("0123456789"),
		WithEntropy(bytes.NewReader(bytes.Repeat([]byte{0xff}, 64))),
	)
	if err != nil {
		t.Fatalf("NewProcessGenerator() error = %v", err)
	}
	var ids []string
	for range times {
		id, err := generator.NewID(KindVersion)
		if err != nil {
			t.Fatalf("NewID() error = %v", err)
		}
		ids = append(ids, id)
		if !strings.HasPrefix(id, "ver_") {
			t.Fatalf("id = %q, want ver_ prefix", id)
		}
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("ids are not lexically sorted: %v", ids)
	}
}

func TestProcessGeneratorSameNanosecondUsesSequence(t *testing.T) {
	fixed := time.Unix(200, 7).UTC()
	generator, err := NewProcessGenerator(
		WithClock(func() time.Time { return fixed }),
		WithGeneratorID("abcdefghjk"),
		WithEntropy(bytes.NewReader(bytes.Repeat([]byte{0x01}, 64))),
	)
	if err != nil {
		t.Fatalf("NewProcessGenerator() error = %v", err)
	}
	first, err := generator.NewID(KindBucket)
	if err != nil {
		t.Fatalf("NewID(first) error = %v", err)
	}
	second, err := generator.NewID(KindBucket)
	if err != nil {
		t.Fatalf("NewID(second) error = %v", err)
	}
	if first >= second {
		t.Fatalf("same-ns ids are not sequence ordered: %q >= %q", first, second)
	}
	if !strings.Contains(first, "_0000_") || !strings.Contains(second, "_0001_") {
		t.Fatalf("sequence fields = %q / %q, want 0000 then 0001", first, second)
	}
}

func TestDeterministicGeneratorReturnsConfiguredIDsByKind(t *testing.T) {
	generator := NewDeterministicGenerator(map[Kind][]string{
		KindBucket:  {"bkt_test_1"},
		KindVersion: {"ver_test_1", "ver_test_2"},
	})
	gotBucket, err := generator.NewID(KindBucket)
	if err != nil {
		t.Fatalf("NewID(bucket) error = %v", err)
	}
	if gotBucket != "bkt_test_1" {
		t.Fatalf("bucket id = %q", gotBucket)
	}
	gotVersion1, err := generator.NewID(KindVersion)
	if err != nil {
		t.Fatalf("NewID(version1) error = %v", err)
	}
	gotVersion2, err := generator.NewID(KindVersion)
	if err != nil {
		t.Fatalf("NewID(version2) error = %v", err)
	}
	if gotVersion1 != "ver_test_1" || gotVersion2 != "ver_test_2" {
		t.Fatalf("version ids = %q/%q", gotVersion1, gotVersion2)
	}
	if _, err := generator.NewID(KindVersion); err == nil {
		t.Fatal("NewID(exhausted) error = nil")
	}
}

func TestGeneratorRejectsInvalidKind(t *testing.T) {
	generator, err := NewProcessGenerator(
		WithGeneratorID("0123456789"),
		WithEntropy(bytes.NewReader(bytes.Repeat([]byte{0x02}, 64))),
	)
	if err != nil {
		t.Fatalf("NewProcessGenerator() error = %v", err)
	}
	if _, err := generator.NewID(Kind("bad")); err == nil {
		t.Fatal("NewID(invalid kind) error = nil")
	}
}
