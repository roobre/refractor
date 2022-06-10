package names

import (
	"github.com/yelinaung/go-haikunator"
	"time"
)

func Haiku() string {
	// Perhaps surprisingly, time.Now().UTC().UnixNano() leads to less repeated numbers than unseeded math.rand.Int63()
	h := haikunator.New(time.Now().UTC().UnixNano())
	return h.Haikunate()
}
