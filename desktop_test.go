package recorder

import (
	"strings"
	"testing"
)

func TestRefuseHeldModifiers(t *testing.T) {
	for _, key := range []uint16{0x10, 0xa0, 0xa1, 0x11, 0xa2, 0xa3, 0x12, 0xa4, 0xa5} {
		err := refuseHeldModifiers(func(candidate uint16) bool { return candidate == key })
		if err == nil {
			t.Fatalf("held modifier 0x%x was accepted", key)
		}
		if !strings.Contains(err.Error(), "release") {
			t.Fatalf("error does not tell the user how to retry: %v", err)
		}
	}
	if err := refuseHeldModifiers(func(uint16) bool { return false }); err != nil {
		t.Fatalf("no held modifiers: %v", err)
	}
}
