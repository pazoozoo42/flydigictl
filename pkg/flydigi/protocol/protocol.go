package protocol

import (
	"context"
	"errors"
)

var (
	ErrUnknownCommand    = errors.New("unknown command type")
	ErrUnknownMessage    = errors.New("unknown message")
	ErrGamepadNotPresent = errors.New("gamepad not present")
)

type Message interface {
	message()
}

type Command interface {
	command()
}

type Protocol interface {
	Close() error

	Messages() <-chan Message
	Send(ctx context.Context, cmd Command) error
}
