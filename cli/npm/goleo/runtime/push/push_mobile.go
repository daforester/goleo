//go:build (android || ios) && goleo_push

package push

import "errors"

var errNoProvider = errors.New("push: no native provider registered: the mobile shell must call SetProvider at startup")

func platformSubscribe(serverKey string) (*PushSubscription, error) {
	return nil, errNoProvider
}

func platformUnsubscribe() error {
	return errNoProvider
}

func platformGetSubscription() (*PushSubscription, error) {
	return nil, errNoProvider
}
