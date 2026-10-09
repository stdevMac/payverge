package formatters

import (
	"bytes"
	"fmt"
	"html/template"
)

var receiptTemplateCompiled = template.Must(template.New("receipt").Funcs(template.FuncMap{
	"money": func(amount float64) string { return fmt.Sprintf("%.2f", amount) },
	// #825: the label column already prints the type word; the value must
	// not repeat it ("Table" | "Table 4").
	"entityValue": EntityValue,
}).Parse(receiptTemplate))

// ReceiptInput extends BillInput with payment-side fields.
type ReceiptInput struct {
	BillInput
	TipAmount     float64
	PaymentMethod string
	TransactionID string
	TotalPaid     float64
	// Fiscal, when non-nil, appends the AFIP/ARCA fiscal block (CAE, número,
	// QR, RG 5614 legend) making the printed ticket a legal comprobante.
	Fiscal *FiscalTicketInfo
}

// FiscalTicketInfo carries the legal AFIP/ARCA fields for a fiscal thermal
// ticket. All values are preformatted strings — the formatter renders them
// verbatim. IVAContained non-empty enables the RG 5614/2024 transparency
// sub-block (B-type letters only; the builder decides). QRDataURI is a
// data:image/png;base64 URI typed template.URL because html/template rewrites
// data: URLs in plain strings to "#ZgotmplZ".
type FiscalTicketInfo struct {
	ReceiptTitle  string // e.g. "FACTURA B"
	ReceiptNumber string // e.g. "0003-00000042"
	EmitterCUIT   string
	EmitterIVA    string // humanized emitter tax condition
	CAE           string
	CAEExpiry     string // DD/MM/YYYY
	IVAContained  string // preformatted, e.g. "ARS 210,00"
	OtherTaxes    string // preformatted, e.g. "ARS 0,00"
	QRDataURI     template.URL
}

// Receipt renders the post-payment receipt as HTML.
func Receipt(in ReceiptInput, paperWidthMM int) (string, error) {
	if paperWidthMM != 80 && paperWidthMM != 58 {
		return "", fmt.Errorf("unsupported paper width: %dmm", paperWidthMM)
	}
	fiscalExtraMM := 0
	if in.Fiscal != nil {
		fiscalExtraMM = 62 // 6 identity/CAE lines + RG 5614 block + 22mm QR
	}
	var buf bytes.Buffer
	buf.Grow(1024 + len(in.Items)*96)
	if err := receiptTemplateCompiled.Execute(&buf, struct {
		ReceiptInput
		PageWidthMM  int
		PageHeightMM int
		PaddingMM    string
		L            printLabels
	}{
		ReceiptInput: in,
		PageWidthMM:  paperWidthMM,
		PageHeightMM: thermalPageHeight(paperWidthMM, in.Items, 108+fiscalExtraMM, in.BusinessAddress, in.TransactionID),
		PaddingMM:    thermalPadding(paperWidthMM),
		L:            labelsFor(in.Language),
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

const receiptTemplate = `<!doctype html>
<html lang="{{.Language}}"><head><meta charset="utf-8">
<style>
  @page { size: {{.PageWidthMM}}mm {{.PageHeightMM}}mm; margin: 0; }
  * { box-sizing: border-box; }
  html, body { width: {{.PageWidthMM}}mm; min-width: {{.PageWidthMM}}mm; max-width: {{.PageWidthMM}}mm; margin: 0; padding: 0; background: #fff; color: #000; }
  body { padding: {{.PaddingMM}}mm; font-family: ui-monospace, "SFMono-Regular", Consolas, monospace; font-size: 11px; line-height: 1.35; -webkit-print-color-adjust: exact; print-color-adjust: exact; }
  main { width: 100%; }
  h1 { margin: 0; text-align: center; font-family: Arial, sans-serif; font-size: 18px; line-height: 1.15; overflow-wrap: anywhere; }
  .address, .document-type, .paid, .footer { text-align: center; overflow-wrap: anywhere; }
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
  .transaction { display: block; margin-top: 2px; font-size: 9px; line-height: 1.25; overflow-wrap: anywhere; word-break: break-word; }
  .sep { border-top: 1px dashed #000; margin: 7px 0; }
  .total { margin-top: 5px; padding-top: 4px; border-top: 2px solid #000; font-size: 13px; font-weight: 800; }
  .paid { margin-top: 8px; border: 2px solid #000; padding: 4px; font-size: 12px; font-weight: 800; letter-spacing: .12em; }
  .footer { margin-top: 7px; font-size: 10px; font-weight: 700; }
  .fiscal { margin-top: 7px; }
  .fiscal-title { text-align: center; font-size: 12px; font-weight: 800; letter-spacing: .06em; }
  .fiscal-legend { margin-top: 4px; text-align: center; font-size: 9px; font-weight: 700; overflow-wrap: anywhere; }
  .fiscal-qr { margin-top: 5px; text-align: center; }
  .fiscal-qr img { width: 22mm; height: 22mm; }
</style></head><body><main>
<h1>{{.BusinessName}}</h1>
{{if .BusinessAddress}}<div class="address">{{.BusinessAddress}}</div>{{end}}
<div class="document-type">{{.L.Receipt}}</div>
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
{{if gt .TipAmount 0.0}}<div class="row"><span class="label">{{.L.Tip}}</span><span class="amount">{{money .TipAmount}}</span></div>{{end}}
<div class="row total"><span>{{.L.TotalPaid}} {{.Currency}}</span><span class="amount">{{money .TotalPaid}}</span></div>
<div class="sep"></div>
<div class="row"><span class="label">{{.L.Method}}</span><span class="value">{{.PaymentMethod}}</span></div>
{{if .TransactionID}}<div><span class="label">{{.L.Txn}}</span><span class="transaction">{{.TransactionID}}</span></div>{{end}}
{{if .Fiscal}}<div class="sep"></div>
<div class="fiscal">
<div class="fiscal-title">{{.Fiscal.ReceiptTitle}}</div>
<div class="row"><span class="label">Nro.</span><span class="value">{{.Fiscal.ReceiptNumber}}</span></div>
<div class="row"><span class="label">CUIT</span><span class="value">{{.Fiscal.EmitterCUIT}}</span></div>
{{if .Fiscal.EmitterIVA}}<div class="row"><span class="label">IVA</span><span class="value">{{.Fiscal.EmitterIVA}}</span></div>{{end}}
<div class="row"><span class="label">CAE</span><span class="value">{{.Fiscal.CAE}}</span></div>
<div class="row"><span class="label">Vto. CAE</span><span class="value">{{.Fiscal.CAEExpiry}}</span></div>
{{if .Fiscal.IVAContained}}<div class="fiscal-legend">Régimen de Transparencia Fiscal al Consumidor (Ley 27.743)</div>
<div class="row"><span class="label">IVA Contenido</span><span class="amount">{{.Fiscal.IVAContained}}</span></div>
<div class="row"><span class="label">Otros Imp. Nac. Indirectos</span><span class="amount">{{.Fiscal.OtherTaxes}}</span></div>{{end}}
{{if .Fiscal.QRDataURI}}<div class="fiscal-qr"><img src="{{.Fiscal.QRDataURI}}" alt="QR ARCA"></div>{{end}}
</div>
{{end}}
<div class="paid">{{.L.Paid}}</div>
<div class="footer">{{.L.ThankYou}}</div>
</main></body></html>`
