//go:build !darwin && !linux && !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("RESULT: SKIP — secure-context spike only targets windows, darwin, linux")
	os.Exit(0)
}
