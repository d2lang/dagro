package dagro

import "testing"

func TestVersionMatchesPortedDagreRelease(t *testing.T) {
	if Version != "3.1.1" {
		t.Fatalf("Version = %q, want %q", Version, "3.1.1")
	}
	if GraphlibVersion != "4.0.5" {
		t.Fatalf("GraphlibVersion = %q, want %q", GraphlibVersion, "4.0.5")
	}
}
