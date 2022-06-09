package pool

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

var errPeek = errors.New("failed to peek body")

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
		go func() {
			// Purge channel when it eventually finishes
			<-result
		}()
		return nil, fmt.Errorf("%w: could not read %dKiB within %.2f seconds", errPeek, p.SizeBytes>>10, p.Timeout.Seconds())
	case result := <-result:
		return result.buf, result.err
	}
}

func (p *Peeker) readPeek(body io.Reader, result chan peekResult) {
	var buf []byte
	var err error

	for {
		readBuf := make([]byte, p.SizeBytes)
		read, err := body.Read(readBuf)
		buf = append(buf, readBuf[:read]...)

		if err != nil || int64(len(buf)) >= p.SizeBytes {
			break
		}
	}

	if err != nil && err != io.EOF {
		result <- peekResult{
			buf: nil,
			err: fmt.Errorf("%w: could not read from body: %v", errPeek, err),
		}
		return
	}

	result <- peekResult{
		buf: buf,
		err: nil,
	}
}
