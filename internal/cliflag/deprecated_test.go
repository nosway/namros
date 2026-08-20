package cliflag

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestStringVarWithDeprecatedAlias(t *testing.T) {
	t.Run("canonical flag", func(t *testing.T) {
		var output bytes.Buffer
		var endpoint string
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.SetOutput(&output)
		StringVarWithDeprecatedAlias(fs, &endpoint, "sbs-service-endpoint", "", "SBS service endpoint", "sbs-admin-endpoint")

		if err := fs.Parse([]string{"--sbs-service-endpoint", "sbs.example:9443"}); err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if endpoint != "sbs.example:9443" {
			t.Fatalf("endpoint = %q, want %q", endpoint, "sbs.example:9443")
		}
		if output.Len() != 0 {
			t.Fatalf("canonical flag output = %q, want empty", output.String())
		}
	})

	t.Run("deprecated alias", func(t *testing.T) {
		var output bytes.Buffer
		var endpoint string
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.SetOutput(&output)
		StringVarWithDeprecatedAlias(fs, &endpoint, "sbs-service-endpoint", "", "SBS service endpoint", "sbs-admin-endpoint")

		if err := fs.Parse([]string{"--sbs-admin-endpoint=sbs.example:9443"}); err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if endpoint != "sbs.example:9443" {
			t.Fatalf("endpoint = %q, want %q", endpoint, "sbs.example:9443")
		}
		if got := output.String(); !strings.Contains(got, "--sbs-admin-endpoint is deprecated; use --sbs-service-endpoint instead") {
			t.Fatalf("deprecated flag output = %q", got)
		}
	})
}
