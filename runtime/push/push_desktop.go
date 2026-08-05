//go:build !android && !ios

package push

import (
	"errors"
	"fmt"
)

// No unified desktop push service exists; use the app's WebSocket channel
// for server-initiated events instead.
var errUnsupported = fmt.Errorf("push: %w on desktop (use the app WebSocket channel)", errors.ErrUnsupported)

func platformSubscribe(serverKey string) (*PushSubscription, error) {
	return nil, errUnsupported
}

func platformUnsubscribe() error {
	return errUnsupported
}

func platformGetSubscription() (*PushSubscription, error) {
	return nil, errUnsupported
}
