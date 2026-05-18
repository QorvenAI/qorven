// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// QuoteGenTool generates PDF quotes and invoices from structured input.
// No external dependencies required — uses pure-Go gofpdf library.
type QuoteGenTool struct {
	workspace string
}

// NewQuoteGenTool returns a new QuoteGenTool that saves PDFs to workspace.
func NewQuoteGenTool(workspace string) *QuoteGenTool {
	return &QuoteGenTool{workspace: workspace}
}

func (t *QuoteGenTool) Name() string { return "generate_quote" }

func (t *QuoteGenTool) Description() string {
	return "Generate a professional PDF quotation or invoice from structured input. " +
		"Accepts buyer/seller details, line items with quantities and prices, notes, and " +
		"a validity date. Returns the file path to the generated PDF. Supports English " +
		"and other languages (pass language code, e.g. 'en', 'fr', 'de')."
}

func (t *QuoteGenTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{
				"type":        "string",
				"enum":        []string{"quote", "invoice"},
				"description": "Document type: 'quote' for a quotation, 'invoice' for an invoice.",
			},
			"buyer": map[string]any{
				"type":        "object",
				"description": "Buyer / Bill-To details.",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string", "description": "Contact name."},
					"company": map[string]any{"type": "string", "description": "Company name."},
					"address": map[string]any{"type": "string", "description": "Full postal address."},
					"email":   map[string]any{"type": "string", "description": "Email address."},
				},
			},
			"seller": map[string]any{
				"type":        "object",
				"description": "Seller / From details.",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string", "description": "Contact name."},
					"company": map[string]any{"type": "string", "description": "Company name."},
					"address": map[string]any{"type": "string", "description": "Full postal address."},
				},
			},
			"line_items": map[string]any{
				"type":        "array",
				"description": "List of products or services being quoted.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"description": map[string]any{"type": "string", "description": "Item description."},
						"qty":         map[string]any{"type": "number", "description": "Quantity."},
						"unit_price":  map[string]any{"type": "number", "description": "Price per unit."},
						"currency":    map[string]any{"type": "string", "description": "ISO currency code, e.g. USD, EUR, INR."},
					},
					"required": []string{"description", "qty", "unit_price"},
				},
			},
			"notes": map[string]any{
				"type":        "string",
				"description": "Optional notes, payment terms, or special conditions.",
			},
			"valid_until": map[string]any{
				"type":        "string",
				"description": "Quote validity date in YYYY-MM-DD format (for quotes only).",
			},
			"language": map[string]any{
				"type":        "string",
				"description": "Language code for labels (default: 'en'). Currently 'en' is fully supported.",
			},
			"reference": map[string]any{
				"type":        "string",
				"description": "Optional document reference / PO number.",
			},
		},
		"required": []string{"type", "buyer", "seller", "line_items"},
	}
}

// quoteLabels holds localised label strings.
type quoteLabels struct {
	Title       string
	Date        string
	ValidUntil  string
	Reference   string
	From        string
	BillTo      string
	Description string
	Qty         string
	UnitPrice   string
	Amount      string
	Total       string
	Notes       string
}

func labelsForLang(lang string) quoteLabels {
	switch strings.ToLower(lang) {
	case "fr":
		return quoteLabels{
			Title: "DEVIS", Date: "Date", ValidUntil: "Valable jusqu'au",
			Reference: "Référence", From: "De", BillTo: "Facturer à",
			Description: "Description", Qty: "Qté", UnitPrice: "Prix unitaire",
			Amount: "Montant", Total: "Total", Notes: "Notes",
		}
	case "de":
		return quoteLabels{
			Title: "ANGEBOT", Date: "Datum", ValidUntil: "Gültig bis",
			Reference: "Referenz", From: "Von", BillTo: "Rechnungsempfänger",
			Description: "Beschreibung", Qty: "Menge", UnitPrice: "Einzelpreis",
			Amount: "Betrag", Total: "Gesamt", Notes: "Anmerkungen",
		}
	case "es":
		return quoteLabels{
			Title: "COTIZACIÓN", Date: "Fecha", ValidUntil: "Válido hasta",
			Reference: "Referencia", From: "De", BillTo: "Facturar a",
			Description: "Descripción", Qty: "Cant.", UnitPrice: "Precio unitario",
			Amount: "Importe", Total: "Total", Notes: "Notas",
		}
	default: // "en" and everything else
		return quoteLabels{
			Title: "QUOTATION", Date: "Date", ValidUntil: "Valid Until",
			Reference: "Reference", From: "From", BillTo: "Bill To",
			Description: "Description", Qty: "Qty", UnitPrice: "Unit Price",
			Amount: "Amount", Total: "Total", Notes: "Notes",
		}
	}
}

type lineItem struct {
	Description string
	Qty         float64
	UnitPrice   float64
	Currency    string
}

func (t *QuoteGenTool) Execute(ctx context.Context, args map[string]any) *Result {
	docType, _ := args["type"].(string)
	if docType == "" {
		docType = "quote"
	}

	buyerMap, _ := args["buyer"].(map[string]any)
	sellerMap, _ := args["seller"].(map[string]any)
	if buyerMap == nil {
		buyerMap = map[string]any{}
	}
	if sellerMap == nil {
		sellerMap = map[string]any{}
	}

	notes, _ := args["notes"].(string)
	validUntil, _ := args["valid_until"].(string)
	lang, _ := args["language"].(string)
	if lang == "" {
		lang = "en"
	}
	reference, _ := args["reference"].(string)

	// Parse line items
	var items []lineItem
	if raw, ok := args["line_items"].([]any); ok {
		for _, ri := range raw {
			im, ok := ri.(map[string]any)
			if !ok {
				continue
			}
			desc, _ := im["description"].(string)
			qty, _ := toFloat(im["qty"])
			up, _ := toFloat(im["unit_price"])
			cur, _ := im["currency"].(string)
			if cur == "" {
				cur = "USD"
			}
			items = append(items, lineItem{
				Description: desc,
				Qty:         qty,
				UnitPrice:   up,
				Currency:    strings.ToUpper(cur),
			})
		}
	}
	if len(items) == 0 {
		return ErrorResult("line_items must contain at least one item")
	}

	labels := labelsForLang(lang)
	if docType == "invoice" {
		labels.Title = map[string]string{
			"fr": "FACTURE", "de": "RECHNUNG", "es": "FACTURA",
		}[strings.ToLower(lang)]
		if labels.Title == "" {
			labels.Title = "INVOICE"
		}
	}

	// Determine currency from first item
	currency := items[0].Currency
	var total float64
	for _, it := range items {
		total += it.Qty * it.UnitPrice
	}

	// Build PDF
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.AddPage()

	pageW, _ := pdf.GetPageSize()
	contentW := pageW - 40 // margins 20+20

	// ── Header: title + date ──────────────────────────────────────
	pdf.SetFont("Helvetica", "B", 24)
	pdf.SetTextColor(30, 30, 30)
	pdf.CellFormat(contentW/2, 12, labels.Title, "", 0, "L", false, 0, "")

	dateStr := time.Now().Format("2006-01-02")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(100, 100, 100)
	pdf.CellFormat(contentW/2, 12, labels.Date+": "+dateStr, "", 1, "R", false, 0, "")

	if reference != "" {
		pdf.SetFont("Helvetica", "", 10)
		pdf.CellFormat(contentW/2, 6, "", "", 0, "L", false, 0, "")
		pdf.CellFormat(contentW/2, 6, labels.Reference+": "+reference, "", 1, "R", false, 0, "")
	}
	if validUntil != "" && docType == "quote" {
		pdf.SetFont("Helvetica", "", 10)
		pdf.CellFormat(contentW/2, 6, "", "", 0, "L", false, 0, "")
		pdf.CellFormat(contentW/2, 6, labels.ValidUntil+": "+validUntil, "", 1, "R", false, 0, "")
	}

	pdf.Ln(6)

	// ── Seller / Buyer blocks ──────────────────────────────────────
	halfW := contentW / 2
	yBefore := pdf.GetY()

	// Seller block (left)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(50, 50, 50)
	pdf.CellFormat(halfW, 6, labels.From, "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(60, 60, 60)
	writePartyBlock(pdf, sellerMap, halfW)

	yAfterSeller := pdf.GetY()

	// Buyer block (right — reset Y to top of block)
	pdf.SetXY(20+halfW, yBefore)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(50, 50, 50)
	pdf.CellFormat(halfW, 6, labels.BillTo, "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(60, 60, 60)
	writePartyBlockAt(pdf, buyerMap, halfW, 20+halfW)

	yAfterBuyer := pdf.GetY()
	// Move below both blocks
	if yAfterSeller > yAfterBuyer {
		pdf.SetY(yAfterSeller)
	}

	pdf.Ln(8)

	// ── Line items table ───────────────────────────────────────────
	colDesc := contentW * 0.50
	colQty := contentW * 0.12
	colPrice := contentW * 0.19
	colAmt := contentW * 0.19

	// Table header
	pdf.SetFillColor(240, 240, 245)
	pdf.SetTextColor(40, 40, 40)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(colDesc, 8, labels.Description, "1", 0, "L", true, 0, "")
	pdf.CellFormat(colQty, 8, labels.Qty, "1", 0, "C", true, 0, "")
	pdf.CellFormat(colPrice, 8, labels.UnitPrice, "1", 0, "R", true, 0, "")
	pdf.CellFormat(colAmt, 8, labels.Amount, "1", 1, "R", true, 0, "")

	// Table rows
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetFillColor(255, 255, 255)
	alt := false
	for _, item := range items {
		if alt {
			pdf.SetFillColor(248, 248, 252)
		} else {
			pdf.SetFillColor(255, 255, 255)
		}
		alt = !alt
		amt := item.Qty * item.UnitPrice
		pdf.SetTextColor(40, 40, 40)
		pdf.CellFormat(colDesc, 7, item.Description, "1", 0, "L", true, 0, "")
		pdf.CellFormat(colQty, 7, fmt.Sprintf("%.2f", item.Qty), "1", 0, "C", true, 0, "")
		pdf.CellFormat(colPrice, 7, formatMoney(item.UnitPrice), "1", 0, "R", true, 0, "")
		pdf.CellFormat(colAmt, 7, formatMoney(amt), "1", 1, "R", true, 0, "")
	}

	// Total row
	pdf.SetFillColor(230, 230, 240)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetTextColor(20, 20, 20)
	pdf.CellFormat(colDesc+colQty+colPrice, 9, labels.Total, "1", 0, "R", true, 0, "")
	pdf.CellFormat(colAmt, 9, currency+" "+formatMoney(total), "1", 1, "R", true, 0, "")

	// ── Notes ──────────────────────────────────────────────────────
	if notes != "" {
		pdf.Ln(8)
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetTextColor(50, 50, 50)
		pdf.CellFormat(contentW, 6, labels.Notes, "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetTextColor(70, 70, 70)
		pdf.MultiCell(contentW, 5, notes, "1", "L", false)
	}

	// ── Save to disk ───────────────────────────────────────────────
	outDir := t.workspace
	if outDir == "" {
		outDir = os.TempDir()
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return ErrorResult("create output directory: " + err.Error())
	}

	ts := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s.pdf", docType, ts)
	outPath := filepath.Join(outDir, filename)

	if err := pdf.OutputFileAndClose(outPath); err != nil {
		return ErrorResult("write PDF: " + err.Error())
	}

	msg := fmt.Sprintf("PDF generated: %s\nFile saved to: %s\nTotal: %s %.2f (%d line item(s))",
		filename, outPath, currency, total, len(items))
	return &Result{
		ForLLM:  msg,
		ForUser: msg,
		Media: []MediaFile{
			{Path: outPath, MimeType: "application/pdf"},
		},
	}
}

// writePartyBlock writes a party block (name, company, address, email) at
// the current X position, using the full halfW column.
func writePartyBlock(pdf *gofpdf.Fpdf, party map[string]any, colW float64) {
	writePartyBlockAt(pdf, party, colW, 20)
}

func writePartyBlockAt(pdf *gofpdf.Fpdf, party map[string]any, colW, x float64) {
	fields := []string{
		strVal(party, "company"),
		strVal(party, "name"),
		strVal(party, "address"),
		strVal(party, "email"),
	}
	for _, f := range fields {
		if f == "" {
			continue
		}
		pdf.SetX(x)
		pdf.CellFormat(colW, 5, f, "", 1, "L", false, 0, "")
	}
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func formatMoney(v float64) string {
	return fmt.Sprintf("%.2f", v)
}
