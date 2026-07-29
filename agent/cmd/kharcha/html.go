// SPDX-License-Identifier: Apache-2.0
package main

import (
	"embed"
	"html/template"
	"io"
	"os"
)

//go:embed report.html.tmpl
var reportTemplateFS embed.FS

var reportTemplate = template.Must(
	template.New("report.html.tmpl").
		Funcs(template.FuncMap{"inc": func(i int) int { return i + 1 }}).
		ParseFS(reportTemplateFS, "report.html.tmpl"),
)

type htmlReportData struct {
	Findings  []*Finding
	PriceBook *PriceBook
}

// WriteHTML renders the top n findings as a static HTML table (build
// manual Step 10.1) — the same []Finding PrintTop uses, just for a browser
// instead of a terminal.
func (a *Aggregate) WriteHTML(w io.Writer, n int) error {
	all := a.Findings()
	if n < len(all) {
		all = all[:n]
	}

	return reportTemplate.Execute(w, htmlReportData{
		Findings:  all,
		PriceBook: a.priceBook,
	})
}

// WriteHTMLFile renders and atomically replaces the HTML report at path.
// Writing to a temp file first and renaming avoids a reader ever seeing a
// half-written report.
func (a *Aggregate) WriteHTMLFile(path string, n int) error {
	tmp := path + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	if err := a.WriteHTML(f, n); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}

	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	return os.Rename(tmp, path)
}
