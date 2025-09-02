package cli

import (
	"fmt"
	"strings"
	"time"
)

// Spinner shows an animated spinner until the returned finalize func is called.
// Call the returned function with success=true on success, or false on failure.
// Example:
//
//	stop := cli.Spinner("Requesting suggestions ...")
//	result, err := work()
//	stop(err == nil)
func Spinner(msg string) func(success bool) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	done := make(chan struct{})
	go func() {
		i := 0
		for {
			select {
			case <-done:
				return
			default:
				fmt.Printf("\r%s %s%s%s", frames[i%len(frames)], ColorDim, msg, ColorReset)
				i++
				time.Sleep(90 * time.Millisecond)
			}
		}
	}()
	// Ensure message without trailing spaces for final line alignment
	cleanMsg := strings.TrimSpace(msg)
	return func(success bool) {
		close(done)
		symbol := IconError
		color := ColorRed
		if success {
			symbol = IconSuccess
			color = ColorGreen
		}
		// Clear line before printing final status to avoid artifact frames
		fmt.Printf("\r%s %s%s%s\n", color+symbol+ColorReset, ColorBold, cleanMsg, ColorReset)
	}
}
