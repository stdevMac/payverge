package formatters

import (
	"bytes"
	"fmt"
	"html/template"
	"unicode/utf8"
)

var billTemplateCompiled = template.Must(template.New("bill").Funcs(template.FuncMap{
	"money": func(amount float64) string { return fmt.Sprintf("%.2f", amount) },
	// #825: the label column already prints the type word; the value must
	// not repeat it ("Table" | "Table 4").
	"entityValue": EntityValue,
}).Parse(billTemplate))

// BillLineItem is one line on a printed bill.
type BillLineKind string

const (
	BillLineKindItem     BillLineKind = "item"
	BillLineKindDetail   BillLineKind = "detail"
	BillLineKindDiscount BillLineKind = "discount"
)

type BillLineItem struct {
	Name     string
	Quantity int
	Subtotal float64
	Kind     BillLineKind
}

// BillInput is the data needed to render a pre-bill ticket.
type BillInput struct {
	BusinessName     string
	BusinessAddress  string
	TableName        string
	BillNumber       string
	CreatedAt        string
	Items            []BillLineItem
	Subtotal         float64
	TaxAmount        float64
	ServiceFeeAmount float64
	Total            float64
	Currency         string
	Language         string
}

// Bill renders the pre-bill ticket as HTML sized for the given paper width.
// Supported widths: 80, 58.
func Bill(in BillInput, paperWidthMM int) (string, error) {
	if paperWidthMM != 80 && paperWidthMM != 58 {
		return "", fmt.Errorf("unsupported paper width: %dmm", paperWidthMM)
	}
	var buf bytes.Buffer
	buf.Grow(1024 + len(in.Items)*96)
	if err := billTemplateCompiled.Execute(&buf, struct {
		BillInput
		PageWidthMM  int
		PageHeightMM int
		PaddingMM    string
		L            printLabels
	}{
		BillInput:    in,
		PageWidthMM:  paperWidthMM,
		PageHeightMM: thermalPageHeight(paperWidthMM, in.Items, 82, in.BusinessAddress, ""),
		PaddingMM:    thermalPadding(paperWidthMM),
		L:            labelsFor(in.Language),
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func thermalPadding(mm int) string {
	if mm == 58 {
		return "2.5"
	}
	return "3.5"
}

// widthClass remains the kitchen-ticket layout selector. Bill and receipt
// layouts use explicit numeric dimensions instead.
func widthClass(mm int) string {
	if mm == 58 {
		return "thermal-58mm"
	}
	return "thermal-80mm"
}

func thermalPageHeight(width int, items []BillLineItem, base int, address, transaction string) int {
	charsPerLine := 34
	if width == 58 {
		charsPerLine = 23
	}
	height := base
	if address != "" {
		height += 5 * wrappedLineCount(address, charsPerLine)
	}
	for _, item := range items {
		lines := wrappedLineCount(item.Name, charsPerLine)
		if item.Kind == BillLineKindDetail {
			height += 5 * lines
		} else {
			height += 7 * lines
		}
	}
	if transaction != "" {
		height += 4 * wrappedLineCount(transaction, charsPerLine)
	}
	if height < 90 {
		return 90
	}
	return height
}

func wrappedLineCount(value string, charsPerLine int) int {
	count := utf8.RuneCountInString(value)
	if count == 0 || charsPerLine <= 0 {
		return 1
	}
	return (count + charsPerLine - 1) / charsPerLine
}

const billTemplate = `<!doctype html>
<html lang="{{.Language}}"><head><meta charset="utf-8">
<style>
  @page { size: {{.PageWidthMM}}mm {{.PageHeightMM}}mm; margin: 0; }
  * { box-sizing: border-box; }
  html, body { width: {{.PageWidthMM}}mm; min-width: {{.PageWidthMM}}mm; max-width: {{.PageWidthMM}}mm; margin: 0; padding: 0; background: #fff; color: #000; }
  body { padding: {{.PaddingMM}}mm; font-family: ui-monospace, "SFMono-Regular", Consolas, monospace; font-size: 11px; line-height: 1.35; -webkit-print-color-adjust: exact; print-color-adjust: exact; }
  main { width: 100%; }
  h1 { margin: 0; text-align: center; font-family: Arial, sans-serif; font-size: 18px; line-height: 1.15; overflow-wrap: anywhere; }
  .address, .document-type, .footer { text-align: center; overflow-wrap: anywhere; }
  .address { margin-top: 3px; font-size: 9px; line-height: 1.3; }
  .document-type { margin-top: 5px; font-size: 10px; font-weight: 700; letter-spacing: .08em; }
  .row { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; break-inside: avoid; }
  .row + .row { margin-top: 2px; }
  .label, .amount { flex: 0 0 auto; white-space: nowrap; font-variant-numeric: tabular-nums; }
  .value { min-width: 0; text-align: right; overflow-wrap: anywhere; }
  .item-name { min-width: 0; flex: 1 1 auto; overflow-wrap: anywhere; }
  .item-amount { flex: 0 0 auto; white-space: nowrap; font-variant-numeric: tabular-nums; }
  .detail { padding-left: 8px; color: #333; font-size: 9px; }
  .detail .item-name::before { content: "↳ "; }
  .discount { font-weight: 600; }
  .sep { border-top: 1px dashed #000; margin: 7px 0; }
  .total { margin-top: 5px; padding-top: 4px; border-top: 2px solid #000; font-size: 13px; font-weight: 800; }
  .footer { margin-top: 9px; font-size: 9px; font-weight: 700; letter-spacing: .04em; }
</style></head><body><main>
<h1>{{.BusinessName}}</h1>
{{if .BusinessAddress}}<div class="address">{{.BusinessAddress}}</div>{{end}}
<div class="document-type">{{.L.PreBill}}</div>
<div class="sep"></div>
<div class="row"><span class="label">{{.L.Bill}}</span><span class="value">{{.BillNumber}}</span></div>
<div class="row"><span class="label">{{.L.Table}}</span><span class="value">{{entityValue .L.Table .TableName}}</span></div>
<div class="row"><span class="label">{{.L.Date}}</span><span class="value">{{.CreatedAt}}</span></div>
<div class="sep"></div>
{{range .Items}}{{if eq .Kind "detail"}}<div class="row detail"><span class="item-name">{{.Quantity}}× {{.Name}}</span></div>{{else}}<div class="row item{{if eq .Kind "discount"}} discount{{end}}"><span class="item-name">{{if ne .Kind "discount"}}{{.Quantity}}× {{end}}{{.Name}}</span><span class="item-amount">{{money .Subtotal}}</span></div>{{end}}
{{end}}<div class="sep"></div>
<div class="row"><span class="label">{{.L.Subtotal}}</span><span class="amount">{{money .Subtotal}}</span></div>
{{if gt .TaxAmount 0.0}}<div class="row"><span class="label">{{.L.Tax}}</span><span class="amount">{{money .TaxAmount}}</span></div>{{end}}
{{if gt .ServiceFeeAmount 0.0}}<div class="row"><span class="label">{{.L.Service}}</span><span class="amount">{{money .ServiceFeeAmount}}</span></div>{{end}}
<div class="row total"><span>{{.L.Total}} {{.Currency}}</span><span class="amount">{{money .Total}}</span></div>
<div class="footer">{{.L.ThankYou}}</div>
</main></body></html>`
