package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
)

// writeXLSX writes a one-sheet workbook. Cells are string (inline text, never a formula),
// int64 or float64 (numbers); nil is an empty cell. Max 26 columns.
func writeXLSX(w io.Writer, header []string, rows [][]any) error {
	z := zip.NewWriter(w)
	files := [][2]string{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Transaksi" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`},
	}
	for _, f := range files {
		fw, err := z.Create(f[0])
		if err != nil {
			return err
		}
		io.WriteString(fw, f[1])
	}

	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" state="frozen"/></sheetView></sheetViews><cols><col min="1" max="` + strconv.Itoa(len(header)) + `" width="16" customWidth="1"/></cols><sheetData>`)
	hdr := make([]any, len(header))
	for i, h := range header {
		hdr[i] = h
	}
	for r, row := range append([][]any{hdr}, rows...) {
		fmt.Fprintf(&b, `<row r="%d">`, r+1)
		for c, v := range row {
			ref := string(rune('A'+c)) + strconv.Itoa(r+1)
			switch v := v.(type) {
			case string:
				fmt.Fprintf(&b, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">`, ref)
				xml.EscapeText(&b, []byte(v))
				b.WriteString(`</t></is></c>`)
			case int64:
				fmt.Fprintf(&b, `<c r="%s"><v>%d</v></c>`, ref, v)
			case float64:
				fmt.Fprintf(&b, `<c r="%s"><v>%s</v></c>`, ref, strconv.FormatFloat(v, 'f', -1, 64))
			}
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	fw, err := z.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		return err
	}
	if _, err := b.WriteTo(fw); err != nil {
		return err
	}
	return z.Close()
}
