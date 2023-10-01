package stats

import (
	"io"
	"sync"
)

// ReaderWrapper wraps an io.Reader and calls OnDone with the total number of bytes read from the underlying reader
// when the underlying reader returns the first error, that can be EOF, or when it gets Close()d.
// OnDone is called at most once.
// If the underlying reader implements io.Closer, ReaderWrapper will forward calls to Close() to it. Otherwise, the
// Close() operation always returns nil.
type ReaderWrapper struct {
	Underlying io.Reader
	OnDone     func(totalRead uint64)

	read uint64
	once sync.Once
}

func (w *ReaderWrapper) Read(p []byte) (n int, err error) {
	n, err = w.Underlying.Read(p)
	w.read += uint64(n)

	if err != nil {
		w.report()
	}

	return
}

func (w *ReaderWrapper) Close() (err error) {
	if closer, ok := w.Underlying.(io.Closer); ok {
		err = closer.Close()
	}

	w.report()
	return
}

func (w *ReaderWrapper) report() {
	w.once.Do(func() {
		w.OnDone(w.read)
	})
}
