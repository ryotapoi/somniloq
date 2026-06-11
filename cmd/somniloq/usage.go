package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

func setUsage(fs *flag.FlagSet, description, usage string) {
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "%s\n\nUsage:\n  %s\n\nFlags:\n", description, usage)
		fs.PrintDefaults()
	}
}

// parseFlags parses args with fs (which must use flag.ContinueOnError),
// sending flag output to errOut. On failure the flag package has already
// reported the problem (or --help output) to errOut, so callers must not
// print the returned state again: they return (code, nil) as is.
func parseFlags(fs *flag.FlagSet, errOut io.Writer, args []string) (code int, ok bool) {
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, false
		}
		return 1, false
	}
	return 0, true
}
