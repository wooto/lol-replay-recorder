package recorder

import "fmt"

func validateSlotKeys(keys [10]uint16) error {
	seen := make(map[uint16]bool, len(keys))
	for slot, key := range keys {
		if key == 0 || key > 0xff {
			return fmt.Errorf("%w: slot %d has no supported Windows virtual-key code", ErrHotkeySettings, slot+1)
		}
		if seen[key] {
			return fmt.Errorf("%w: slot %d repeats virtual key %d", ErrHotkeySettings, slot+1, key)
		}
		seen[key] = true
	}
	return nil
}
