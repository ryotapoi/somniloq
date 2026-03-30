package main

import (
	"flag"
	"fmt"
)

func setUsage(fs *flag.FlagSet, description, usage string) {
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "%s\n\nUsage:\n  %s\n\nFlags:\n", description, usage)
		fs.PrintDefaults()
	}
}
