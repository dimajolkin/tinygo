//go:build esp32s3

package main

// ESP32-S3 Pin Configuration for Interrupt Examples
//
// This file contains common configuration for ESP32-S3 interrupt examples.
// Adjust these values based on your specific ESP32-S3 board.

import "machine"

// GPIO Pin Configuration
// These pins are used in the interrupt example
const (
	// Boot Button - GPIO0
	// Standard button on all ESP32-S3 boards
	// Triggered on falling edge (pressed = LOW)
	BUTTON_GPIO = machine.GPIO0

	// LED Pin - GPIO42
	// Some boards use different pins for LED
	// Common alternatives: GPIO2, GPIO38, GPIO48
	LED_GPIO = machine.GPIO42
)

// Interrupt Edge Configuration
// PinFalling = interrupt on falling edge (button press)
// PinRising = interrupt on rising edge (button release)
// PinToggle = interrupt on both edges
var (
	BUTTON_INTERRUPT_MODE = machine.PinFalling
)

// Helper function to get GPIO pin by number
func getGPIO(num uint8) machine.Pin {
	return machine.Pin(num)
}

// Helper function to configure a GPIO pin as input with pullup
func configureButtonPin(pin machine.Pin) {
	pin.Configure(machine.PinConfig{
		Mode: machine.PinInputPullup,
	})
}

// Helper function to configure a GPIO pin as output
func configureLEDPin(pin machine.Pin) {
	pin.Configure(machine.PinConfig{
		Mode: machine.PinOutput,
	})
	pin.High() // Set LED to off (active-low on most boards)
}
