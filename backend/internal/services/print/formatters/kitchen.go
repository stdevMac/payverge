package formatters

import (
	"bytes"
	"fmt"
	"html/template"
)

var kitchenTemplateCompiled = template.Must(template.New("kitchen").Funcs(template.FuncMap{
	"hasItems": func(s []string) bool { return len(s) > 0 },
	// #825: seed names already carry the type word ("Table 1").
	"entityName": EntityName,
}).Parse(kitchenTemplate))

// KitchenLineItem is one line on a kitchen ticket. Modifiers + allergens are
// surfaced per the IMP-13 spec — line cooks need to see add-ons, special
// requests, and allergen flags inline with the dish.
type KitchenLineItem struct {
	Name      string
	Quantity  int
	Modifiers []string // "no onions", "extra sauce" — flat list, label-only
	Notes     string   // free-form per-item ("rare please")
	Allergens []string // EU-14 baseline tags pulled from MenuItem.allergens
}

// KitchenTicketInput is the data needed to render a kitchen ticket. Unlike
// the bill/receipt formatters, money is intentionally omitted — line cooks
// don't care about prices.
type KitchenTicketInput struct {
	BusinessName string
	TableName    string
	BillNumber   string
	OrderNumber  string
	CreatedAt    string
	OrderNotes   string // order-level notes (applies to all items)
	Items        []KitchenLineItem
	Language     string
}

// KitchenTicket renders the kitchen ticket as HTML sized for the given paper
// width. Supported widths: 80, 58.
func KitchenTicket(in KitchenTicketInput, paperWidthMM int) (string, error) {
	if paperWidthMM != 80 && paperWidthMM != 58 {
		return "", fmt.Errorf("unsupported paper width: %dmm", paperWidthMM)
	}
	var buf bytes.Buffer
	buf.Grow(1024 + len(in.Items)*128)
	if err := kitchenTemplateCompiled.Execute(&buf, struct {
		KitchenTicketInput
		WidthClass string
		L          printLabels
	}{in, widthClass(paperWidthMM), labelsFor(in.Language)}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// kitchenTemplate is intentionally stripped-down: bold item, indented
// modifiers/notes, uppercased allergen banner. Mirrors the ESC/POS layout
// described in IMP-13 (32-char width usable on 58mm, more breathing room on
// 80mm).
const kitchenTemplate = `<!doctype html>
<html lang="{{.Language}}"><head><meta charset="utf-8">
<style>
  @page { size: {{if eq .WidthClass "thermal-58mm"}}58mm{{else}}80mm{{end}} auto; margin: 0; }
  body { font-family: ui-monospace, monospace; font-size: 13px; margin: 0; padding: 4mm; }
  h1   { font-size: 16px; text-align: center; margin: 0 0 4px; letter-spacing: 1px; }
  .meta { font-size: 11px; text-align: center; margin-bottom: 4px; }
  .sep { border-top: 1px dashed #000; margin: 6px 0; }
  .item { margin: 4px 0; }
  .item-name { font-weight: 700; font-size: 14px; }
  .mods { padding-left: 8px; font-size: 12px; }
  .mod-line { margin: 0; }
  .notes { padding-left: 8px; font-style: italic; font-size: 12px; }
  .allergens { padding-left: 8px; font-weight: 700; text-transform: uppercase; font-size: 12px; }
  .order-notes { font-weight: 700; text-align: center; padding: 4px 0; }
  .footer { text-align: center; margin-top: 6px; font-size: 11px; }
</style></head><body>
<h1>{{.L.Kitchen}}</h1>
<div class="meta">{{.BusinessName}}</div>
<div class="sep"></div>
<div class="meta">{{.L.OrderNum}}{{.OrderNumber}} &middot; {{.L.Bill}} {{.BillNumber}}</div>
<div class="meta">{{entityName .L.Table .TableName}} &middot; {{.CreatedAt}}</div>
<div class="sep"></div>
{{if .OrderNotes}}<div class="order-notes">** {{.OrderNotes}} **</div><div class="sep"></div>{{end}}
{{range .Items}}<div class="item">
  <div class="item-name">{{.Quantity}}x {{.Name}}</div>
  {{if hasItems .Modifiers}}<div class="mods">{{range .Modifiers}}<div class="mod-line">+ {{.}}</div>{{end}}</div>{{end}}
  {{if .Notes}}<div class="notes">&quot;{{.Notes}}&quot;</div>{{end}}
  {{if hasItems .Allergens}}<div class="allergens">{{$.L.Allergens}} {{range $i, $a := .Allergens}}{{if $i}}, {{end}}{{$a}}{{end}}</div>{{end}}
</div>{{end}}
<div class="sep"></div>
<div class="footer">{{.L.EndOfTicket}}</div>
</body></html>`
