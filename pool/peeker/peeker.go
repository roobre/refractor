package peeker

import (
	"errors"
	"fmt"
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
	result := make(chan peekResult)
	timer := time.NewTimer(p.Timeout)
	defer timer.Stop()

	go p.readPeek(body, result)

	select {
	case <-timer.C:
		return nil, fmt.Errorf("could not read %dKiB within %.2fs: %w", p.SizeBytes>>10, p.Timeout.Seconds(), ErrPeekTimeout)
	case result := <-result:
		return result.buf, result.err
	}
}

func (p *Peeker) readPeek(body io.Reader, resultChan chan peekResult) {
	result := peekResult{}

	read := 0
	for {
		readBuf := make([]byte, p.SizeBytes-int64(read))
		read, result.err = body.Read(readBuf)
		result.buf = append(result.buf, readBuf[:read]...)

		if result.err != nil || int64(len(result.buf)) >= p.SizeBytes {
			break
		}
	}

	select {
	case resultChan <- result:
	default:
		return
	}
}
