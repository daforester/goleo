package runtime

import "os"

func Getenv(key string) string {
	return os.Getenv(key)
}
