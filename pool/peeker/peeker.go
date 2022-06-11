package peeker

import (
	"context"
	"errors"
	"io"
	"time"
)

type Peeker struct {
	SizeBytes int64
	Timeout   time.Duration
}

var ErrPeekTimeout = errors.New("peek timed out")

type peekResult struct {
	buf []byte
	err error
}

// Peek attempts to get a few bytes from the body within some time.
func (p *Peeker) Peek(body io.Reader) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout)
	defer cancel()

	readChan := p.readContext(ctx, body)

	select {
	case result := <-readChan:
		return result.buf, result.err
	case <-ctx.Done():
		return nil, ErrPeekTimeout
	}
}

func (p *Peeker) readContext(ctx context.Context, body io.Reader) chan peekResult {
	res := make(chan peekResult)

	go func() {
		result := peekResult{}
		result.buf, result.err = io.ReadAll(io.LimitReader(body, p.SizeBytes))

		select {
		case <-ctx.Done():
		case res <- result:
		}
	}()

	return res
}
