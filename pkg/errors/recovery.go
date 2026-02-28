package errors

import (
	"fmt"
	"runtime/debug"

	"github.com/ipfs/go-log/v2"
)

var logger = log.Logger("babylontower/recovery")

// SafeGo launches a goroutine with panic recovery. If the function panics,
// the panic is caught and logged with the goroutine name and stack trace.
func SafeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				logger.Errorw("goroutine panic recovered",
					"goroutine", name,
					"panic", fmt.Sprintf("%v", r),
					"stack", string(stack),
				)
			}
		}()
		fn()
	}()
}
