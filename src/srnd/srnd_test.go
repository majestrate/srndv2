package srnd

import "testing"

func TestGenFeedsConfig(t *testing.T) {

	err := GenFeedsConfig()
	// Generate default feeds.ini
	if err != nil {

		t.Error("Cannot generate feeds.ini", err)

	}

}
