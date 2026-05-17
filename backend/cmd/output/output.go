// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Printer struct {
	Format string
}

func NewPrinter(format string) *Printer {
	return &Printer{Format: format}
}

func (p *Printer) Print(data any) {
	switch p.Format {
	case "json":
		p.printJSON(data)
	case "yaml":
		p.printYAML(data)
	default:
		if td, ok := data.(*TableData); ok {
			p.printTable(td)
		} else {
			p.printJSON(data)
		}
	}
}

type TableData struct {
	Headers []string
	Rows    [][]string
}

func NewTable(headers ...string) *TableData {
	return &TableData{Headers: headers}
}

func (td *TableData) AddRow(values ...string) {
	td.Rows = append(td.Rows, values)
}

func (p *Printer) printJSON(data any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
}

func (p *Printer) printYAML(data any) {
	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	_ = enc.Encode(data)
}

func (p *Printer) printTable(td *TableData) {
	if len(td.Rows) == 0 {
		fmt.Println("No results found.")
		return
	}
	widths := make([]int, len(td.Headers))
	for i, h := range td.Headers {
		widths[i] = len(h)
	}
	for _, row := range td.Rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	printRow(td.Headers, widths)
	sep := make([]string, len(widths))
	for i, w := range widths {
		sep[i] = strings.Repeat("-", w)
	}
	fmt.Println(strings.Join(sep, "  "))
	for _, row := range td.Rows {
		printRow(row, widths)
	}
}

func printRow(cells []string, widths []int) {
	parts := make([]string, len(cells))
	for i, cell := range cells {
		w := 0
		if i < len(widths) {
			w = widths[i]
		}
		parts[i] = fmt.Sprintf("%-*s", w, cell)
	}
	fmt.Println(strings.Join(parts, "  "))
}

func (p *Printer) Error(err error) {
	if p.Format == "json" {
		p.printJSON(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	fmt.Fprintf(os.Stderr, "Error: %s\n", err)
}

func (p *Printer) Success(msg string) {
	if p.Format == "json" {
		p.printJSON(map[string]any{"ok": true, "message": msg})
		return
	}
	fmt.Println(msg)
}
