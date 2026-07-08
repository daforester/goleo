//go:build !android && !ios

package vibration

import (
	"errors"
	"fmt"
)

func platformVibrate(pattern []int64) error {
	return fmt.Errorf("vibration: %w on desktop", errors.ErrUnsupported)
}
