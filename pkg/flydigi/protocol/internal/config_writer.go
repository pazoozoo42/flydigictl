package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

type writerContext interface {
	io.Writer

	WriteContext(ctx context.Context, p []byte) (int, error)
}

type ConfigWriter struct {
	w io.Writer

	lock      sync.Mutex
	isSending bool

	chunkack chan int
}

func NewConfigWriter(w io.Writer) *ConfigWriter {
	return &ConfigWriter{
		w:        w,
		chunkack: make(chan int),
	}
}

func (cw *ConfigWriter) Ack(n int) {
	if cw.isSending {
		select {
		case cw.chunkack <- n:
		case <-time.After(1 * time.Second):
		}
	}
}

func (cw *ConfigWriter) Send(ctx context.Context, chunks [][]byte, maxRetries int, chunkTimeout time.Duration) error {
	cw.lock.Lock()
	cw.isSending = true

	defer func() {
		cw.isSending = false
		cw.lock.Unlock()
	}()

	for i, chunk := range chunks {
		retriesLeft := maxRetries
		success := false

		for !success && retriesLeft > 0 {
			retriesLeft--

			var err error
			if ctxw, ok := cw.w.(writerContext); ok {
				_, err = ctxw.WriteContext(ctx, chunk)
			} else {
				_, err = cw.w.Write(chunk)
			}
			if err != nil {
				return fmt.Errorf("write chunk: %w", err)
			}

			for {
				select {
				case ack := <-cw.chunkack:
					if ack < i {
						continue // Invalid ack number
					}
					success = true

				case <-time.After(chunkTimeout):

				case <-ctx.Done():
					return ctx.Err()
				}

				break
			}
		}

		if !success {
			return errors.New("device didn't respond")
		}
	}

	return nil
}
