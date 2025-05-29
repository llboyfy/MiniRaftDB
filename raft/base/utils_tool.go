package base

import (
	"math/rand"
	"time"
)

func RandTimeout(min, max time.Duration, defaults ...bool) time.Duration {
	if len(defaults) > 0 && defaults[0] {
		min = 150 * time.Millisecond
		max = 300 * time.Millisecond
	}
	return min + time.Duration(rand.Int63n(int64(max-min)))
}

func SafeClose(ch *chan struct{}) {
	if *ch != nil {
		select {
		case <-*ch:
			// Already closed
		default:
			close(*ch)
		}
		*ch = nil // Mark as closed
	}
}
