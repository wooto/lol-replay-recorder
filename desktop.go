package recorder

import (
	"errors"
	"strings"
)

var modifierKeys = []struct {
	name string
	keys []uint16
}{
	{"Shift", []uint16{0x10, 0xa0, 0xa1}},
	{"Ctrl", []uint16{0x11, 0xa2, 0xa3}},
	{"Alt", []uint16{0x12, 0xa4, 0xa5}},
}

// refuseHeldModifiers only observes keyboard state. It never changes it.
func refuseHeldModifiers(isDown func(uint16) bool) error {
	var held []string
	for _, modifier := range modifierKeys {
		for _, key := range modifier.keys {
			if isDown(key) {
				held = append(held, modifier.name)
				break
			}
		}
	}
	if len(held) == 0 {
		return nil
	}
	return errors.New("camera selection paused because a modifier key is held: " + strings.Join(held, ", ") + "; release it and retry")
}
