package dagro

import "testing"

func TestVersionMatchesPortedDagreRelease(t *testing.T) {
	if Version != "0.8.5" {
		t.Fatalf("Version = %q, want %q", Version, "0.8.5")
	}
	if GraphlibVersion != "2.1.8" {
		t.Fatalf("GraphlibVersion = %q, want %q", GraphlibVersion, "2.1.8")
	}
}
