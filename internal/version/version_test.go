package version

import (
	"os"
	"testing"
)

func TestInfoReflectsLinkerVariables(t *testing.T) {
	info := Info()
	for key, value := range map[string]string{
		"version": Version,
		"commit":  Commit,
		"date":    Date,
	} {
		if info[key] != value {
			t.Fatalf("Info()[%q] = %q, want %q", key, info[key], value)
		}
	}

	if want := os.Getenv("NAMROS_VERSION_TEST_WANT_VERSION"); want != "" && Version != want {
		t.Fatalf("Version = %q, want %q", Version, want)
	}
	if want := os.Getenv("NAMROS_VERSION_TEST_WANT_COMMIT"); want != "" && Commit != want {
		t.Fatalf("Commit = %q, want %q", Commit, want)
	}
	if want := os.Getenv("NAMROS_VERSION_TEST_WANT_DATE"); want != "" && Date != want {
		t.Fatalf("Date = %q, want %q", Date, want)
	}
}
