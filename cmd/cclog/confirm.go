package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func confirmFullImport(in io.Reader, out io.Writer) bool {
	fmt.Fprint(out, "This will delete all data and re-import. Continue? [y/N] ")
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(scanner.Text()), "y")
}
