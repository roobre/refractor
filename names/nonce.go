package names

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io"
	"strings"
	"time"
)

func Nonce() string {
	nonce := make([]byte, 5)
	_, err := io.ReadFull(rand.Reader, nonce)
	if err != nil {
		time.Sleep(time.Nanosecond)
		return fmt.Sprint(time.Now().Nanosecond())
	}

	return strings.ToLower(base32.StdEncoding.EncodeToString(nonce))
}
