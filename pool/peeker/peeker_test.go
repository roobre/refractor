package peeker_test

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"roob.re/refractor/pool/peeker"
	"strings"
	"testing"
	"time"
)

const (
	part1 = "lorem ipsum "
	part2 = "dolor sit amet"
	full  = "lorem ipsum dolor sit amet"
)

func TestPeeker_Peeks_Some_Bytes(t *testing.T) {
	t.Parallel()

	reader := strings.NewReader(full)
	pk := peeker.Peeker{
		SizeBytes: int64(len(part1)),
		Timeout:   1 * time.Second,
	}

	read, err := pk.Peek(reader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(read, []byte(part1)) {
		t.Fatal("read is expected to be part1")
	}

	rest, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(rest, []byte(part2)) {
		t.Fatal("read is expected to be part1")
	}
}

func TestPeeker_Peeks(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		len  int
	}{
		{name: "All_Bytes", len: len(full)},
		{name: "No_Extra_Bytes", len: len(full) + 10},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			reader := strings.NewReader(full)
			peeker := peeker.Peeker{
				SizeBytes: int64(tc.len),
				Timeout:   1 * time.Second,
			}

			read, err := peeker.Peek(reader)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(read, []byte(full)) {
				t.Fatal("read is expected to be part1")
			}

			rest, err := ioutil.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}

			if len(rest) != 0 {
				t.Fatal("read is expected to be part1")
			}
		})
	}
}

type delayReader struct {
	io.Reader
}

func (dr delayReader) Read(buf []byte) (int, error) {
	time.Sleep(1 * time.Second)
	return dr.Reader.Read(buf)
}

func TestPeeker_Times_Out(t *testing.T) {
	t.Parallel()

	reader := delayReader{strings.NewReader(full)}
	pk := peeker.Peeker{
		SizeBytes: int64(len(part1)),
		Timeout:   500 * time.Millisecond,
	}

	read, err := pk.Peek(reader)
	if !errors.Is(err, peeker.ErrPeekTimeout) {
		t.Fatal("peeker did not time out")
	}

	if len(read) != 0 {
		t.Fatal("peeker returned more than 0 bytes on timeout")
	}

	time.Sleep(1 * time.Second)

	rest, err := io.ReadAll(reader)
	if !bytes.Equal(rest, []byte(part2)) {
		t.Fatal("peeker left unexpected stuf fin the buffer")
	}
}
