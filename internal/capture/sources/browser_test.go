package sources

import (
	"os"
	"testing"
)

func TestChromeEpochRoundtrips(t *testing.T) {
	b := Browser{Flavor: Chromium}
	const unix int64 = 1_752_883_200
	native := (unix + chromeEpochOffsetS) * 1_000_000
	if got := b.toUnix(native); got != unix {
		t.Errorf("toUnix = %d, want %d", got, unix)
	}
}

func TestSafariEpochRoundtrips(t *testing.T) {
	b := Browser{Flavor: Safari}
	const unix int64 = 1_752_883_200
	native := unix - safariEpochOffsetS
	if got := b.toUnix(native); got != unix {
		t.Errorf("toUnix = %d, want %d", got, unix)
	}
}

func TestDetectReturnsOnlyExistingFiles(t *testing.T) {
	for _, b := range DetectBrowsers() {
		if _, err := os.Stat(b.DB); err != nil {
			t.Errorf("%s reported without a history file: %v", b.Name, err)
		}
	}
}
