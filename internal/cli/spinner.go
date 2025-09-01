package cli

import (
	"fmt"
	"time"
)

// Spinner returns a stop function that finalizes the spinner.
func Spinner(msg string) func() {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	done := make(chan struct{})
	go func() {
		i := 0
		for {
			select {
			case <-done:
				return
			default:
				fmt.Printf("\r%s %s", frames[i%len(frames)], msg)
				i++
				time.Sleep(90 * time.Millisecond)
			}
		}
	}()
	return func() {
		close(done)
		fmt.Printf("\r✓ %s\n", msg)
	}
}
