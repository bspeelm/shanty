//go:build flash

package sky

import "testing"

// TestNoSeedFlashesMoreThanTwiceASecond is the full sweep: sixty seeds for a
// minute each, at 120 by 40. It runs with -tags=flash.
func TestNoSeedFlashesMoreThanTwiceASecond(t *testing.T) {
	sweepFlashes(t, 60, 60, 120, 40)
}
