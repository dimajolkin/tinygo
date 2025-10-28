//go:build esp32s3

package main

import "device/esp"

func main() {
	println("Checking vectors...")
	_ = esp.VectorEntryCount
}

