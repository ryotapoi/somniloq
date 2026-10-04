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
	metadata := struct {
		Total      int  `json:"total"`
		Count      int  `json:"count"`
		Limit      *int `json:"limit"`
		Offset     int  `json:"offset"`
		HasMore    bool `json:"hasMore"`
		NextOffset *int `json:"nextOffset"`
	}{page.Total, page.Count, page.Limit, page.Offset, page.HasMore, page.NextOffset}
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(out, "# page\t%s\nref\tmessageNumber\trole\ttimestamp\ttext\tblocks\tparentRef\trootRef\tprovenance\n", data); err != nil {
		return err
	}
	for _, m := range page.Items {
		blocks := `\N`
		if m.Blocks != nil {
			data, err = json.Marshal(m.Blocks)
			if err != nil {
				return err
			}
			blocks = string(data)
		}
		fields := []string{showTSVString(m.REF), strconv.Itoa(m.MessageNumber), showTSVString(m.Role), showTSVNullable(m.Timestamp), showTSVString(m.Text), blocks, showTSVNullable(m.ParentREF), showTSVNullable(m.RootREF), showTSVString(m.Provenance)}
		if _, err = fmt.Fprintln(out, strings.Join(fields, "\t")); err != nil {
			return err
		}
	}
	return nil
}
