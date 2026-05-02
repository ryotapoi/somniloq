package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// confirmYesNo writes prompt to out and reads one line from in. Returns true
// only when the response (after trimming) equals "y" or "Y". EOF, empty input,
// and any other text returns false.
func confirmYesNo(in io.Reader, out io.Writer, prompt string) bool {
	fmt.Fprint(out, prompt)
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(scanner.Text()), "y")
}

func confirmFullImport(in io.Reader, out io.Writer) bool {
	return confirmYesNo(in, out, "This will delete all data and re-import. Continue? [y/N] ")
}

func confirmBackfillDelete(in io.Reader, out io.Writer, count int) bool {
	return confirmYesNo(in, out, fmt.Sprintf("This will delete %d session(s) with no messages. Continue? [y/N] ", count))
}
