package dagro

import (
	"os"
	"testing"
)

// slowUnderRace skips t in a -race build unless DAGRO_SLOW=1, so that
// the package's tests finish within a few minutes with -race.
// It marks single-goroutine differential tests that take 20-50 s under the
// race detector on an idle machine and have no concurrency for it to check;
// they still run in every build without -race.
func slowUnderRace(t *testing.T) {
	t.Helper()
	if raceEnabled && os.Getenv("DAGRO_SLOW") == "" {
		t.Skip("single goroutine, slow under -race: set DAGRO_SLOW=1")
	}
}
