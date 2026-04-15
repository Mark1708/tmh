package cmd

import "fmt"

// cmdErr is a tiny alias used to keep subcommand bodies tight and consistent.
func cmdErr(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
