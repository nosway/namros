package mcpops

import (
	"os"
	"testing"

	"github.com/nosway/namros/internal/edition"
)

func enterpriseOverlayTest() bool {
	return os.Getenv("NAMROS_ENTERPRISE_OVERLAY_TEST") == "1" && edition.Current() == edition.Enterprise
}

func skipEnterpriseOverlayCommunityAssertion(t *testing.T) {
	t.Helper()
	if enterpriseOverlayTest() {
		t.Skip("community edition assertion is not applicable to Enterprise overlay test runs")
	}
}
