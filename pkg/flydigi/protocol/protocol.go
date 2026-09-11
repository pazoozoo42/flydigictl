package protocol

import (
	"context"
	"errors"
)

var (
	ErrUnknownCommand    = errors.New("unknown command type")
	ErrUnknownMessage    = errors.New("unknown message")
	ErrGamepadNotPresent = errors.New("gamepad not present")
	ErrUnsupported       = errors.New("operation not supported by this gamepad or connection mode")
)

// Version identifies the wire protocol family spoken by a gamepad.
type Version int

const (
	// VersionV1 is the legacy protocol used by Vader 2/3/4, Apex 2/3/4 and Direwolf
	// controllers (Cypress VID 04b4 in DInput mode, Xbox 360 VID 045e in XInput mode).
	VersionV1 Version = 1
	// VersionV2 is the "NewXInput" protocol used by controllers with Flydigi's own
	// VID 37d7 (Vader 5 Pro, Apex 5, Apex 6): 32-byte reports framed with 5A A5.
	VersionV2 Version = 2
)

type Message interface {
	message()
}

type Command interface {
	command()
}

// USBLocator is implemented by protocols that know which USB device they talk to.
type USBLocator interface {
	// USBDevicePath returns the sysfs directory of the USB device, or "" if unknown.
	USBDevicePath() string
}

type Protocol interface {
	Close() error

	// Version reports the protocol family in use.
	Version() Version

	Messages() <-chan Message
	Send(ctx context.Context, cmd Command) error
}
