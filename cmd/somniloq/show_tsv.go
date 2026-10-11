package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func showTSVString(value string) string {
	escapes := []string{"\\", "\\\\", "\t", "\\t", "\n", "\\n", "\r", "\\r"}
	for r := rune(0); r <= 0x9f; r++ {
		if r < 0x20 || r >= 0x7f {
			escapes = append(escapes, string(r), fmt.Sprintf(`\x%02x`, r))
		}
	}
	return strings.NewReplacer(escapes...).Replace(value)
}

// JSON already escapes C0; escape DEL/C1 without changing JSON string values.
func showTSVJSON(data []byte) string {
	escapes := []string{}
	for r := rune(0x7f); r <= 0x9f; r++ {
		escapes = append(escapes, string(r), fmt.Sprintf(`\u%04x`, r))
	}
	return strings.NewReplacer(escapes...).Replace(string(data))
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
			blocks = showTSVJSON(data)
		}
		fields := []string{showTSVString(m.REF), strconv.Itoa(m.MessageNumber), showTSVString(m.Role), showTSVNullable(m.Timestamp), showTSVString(m.Text), blocks, showTSVNullable(m.ParentREF), showTSVNullable(m.RootREF), showTSVString(m.Provenance)}
		if _, err := fmt.Fprintln(out, strings.Join(fields, "\t")); err != nil {
			return err
		}
	}
	return nil
}
