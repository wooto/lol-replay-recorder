package lcu

import "testing"

func TestReplayPlatform(t *testing.T) {
	for region, want := range map[string]string{"NA": "NA1", "NA1": "NA1", "EUW": "EUW1", "EUW1": "EUW1", "VN": "VN2", "VN2": "VN2", "KR": "KR", "EUNE": "EUN1"} {
		if got := replayPlatform(region); got != want {
			t.Errorf("%s: got %s, want %s", region, got, want)
		}
	}
}
