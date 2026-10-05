package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func showTSVString(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\t", "\\t", "\n", "\\n", "\r", "\\r").Replace(value)
}
func showTSVNullable(value *string) string {
	if value == nil {
		return `\N`
	}
	return showTSVString(*value)
}
func writeShowTSV(out io.Writer, page showJSON) error {
	if err := writePageTSVHeader(out, page, "ref\tmessageNumber\trole\ttimestamp\ttext\tblocks\tparentRef\trootRef\tprovenance"); err != nil {
		return err
	}
	for _, m := range page.Items {
		blocks := `\N`
		if m.Blocks != nil {
			data, err := json.Marshal(m.Blocks)
			if err != nil {
				return err
			}
			blocks = string(data)
		}
		fields := []string{showTSVString(m.REF), strconv.Itoa(m.MessageNumber), showTSVString(m.Role), showTSVNullable(m.Timestamp), showTSVString(m.Text), blocks, showTSVNullable(m.ParentREF), showTSVNullable(m.RootREF), showTSVString(m.Provenance)}
		if _, err := fmt.Fprintln(out, strings.Join(fields, "\t")); err != nil {
			return err
		}
	}
	return nil
}
