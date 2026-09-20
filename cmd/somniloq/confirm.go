package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// confirmYesNo writes prompt to out and reads one line from in. It returns true
// only when the response (after trimming) equals "y" or "Y". EOF, empty input,
// and any other text return false without an error.
func confirmYesNo(in io.Reader, out io.Writer, prompt string) (bool, error) {
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return false, err
	}
	scanner := bufio.NewScanner(in)
	confirmed := scanner.Scan()
	if err := scanner.Err(); err != nil {
		return false, err
	}
	if !confirmed {
		return false, nil
	}
	return strings.EqualFold(strings.TrimSpace(scanner.Text()), "y"), nil
}

func confirmFullImport(in io.Reader, out io.Writer) (bool, error) {
	return confirmYesNo(in, out, "This will delete all data and re-import. Continue? [y/N] ")
}

func confirmBackfillDelete(in io.Reader, out io.Writer, count int) (bool, error) {
	return confirmYesNo(in, out, fmt.Sprintf("This will delete %d session(s) with no messages. Continue? [y/N] ", count))
}
