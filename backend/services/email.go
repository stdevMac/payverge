package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"time"

	"invoice-generator/models"
)

type EmailService struct {
	apiKey     string
	baseURL    string
	fromEmail  string
	fromName   string
	httpClient *http.Client
}

type PostmarkEmail struct {
	From     string `json:"From"`
	To       string `json:"To"`
	Subject  string `json:"Subject"`
	HtmlBody string `json:"HtmlBody"`
	TextBody string `json:"TextBody"`
	Tag      string `json:"Tag"`
}

type PostmarkResponse struct {
	To          string `json:"To"`
	SubmittedAt string `json:"SubmittedAt"`
	MessageID   string `json:"MessageID"`
	ErrorCode   int    `json:"ErrorCode"`
	Message     string `json:"Message"`
}

func NewEmailService() *EmailService {
	return &EmailService{
		apiKey:     os.Getenv("POSTMARK_API_KEY"),
		baseURL:    "https://api.postmarkapp.com",
		fromEmail:  getEnvOrDefault("FROM_EMAIL", "invoices@yourdomain.com"),
		fromName:   getEnvOrDefault("FROM_NAME", "Invoice Generator"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (es *EmailService) SendInvoiceEmail(invoice *models.Invoice) error {
	if invoice.PayerEmail == "" {
		return fmt.Errorf("no payer email provided")
	}

	subject := fmt.Sprintf("Invoice #%d from %s", invoice.InvoiceID, invoice.CreatorName)
	if invoice.CreatorName == "" {
		subject = fmt.Sprintf("Invoice #%d", invoice.InvoiceID)
	}

	htmlBody, textBody, err := es.generateInvoiceEmailContent(invoice)
	if err != nil {
		return fmt.Errorf("failed to generate email content: %v", err)
	}

	email := PostmarkEmail{
		From:     fmt.Sprintf("%s <%s>", es.fromName, es.fromEmail),
		To:       invoice.PayerEmail,
		Subject:  subject,
		HtmlBody: htmlBody,
		TextBody: textBody,
		Tag:      "invoice-initial",
	}

	return es.sendEmail(email)
}

func (es *EmailService) SendReminderEmail(invoice *models.Invoice) error {
	if invoice.PayerEmail == "" {
		return fmt.Errorf("no payer email provided")
	}

	subject := fmt.Sprintf("Reminder: Invoice #%d Payment Due", invoice.InvoiceID)

	htmlBody, textBody, err := es.generateReminderEmailContent(invoice)
	if err != nil {
		return fmt.Errorf("failed to generate reminder email content: %v", err)
	}

	email := PostmarkEmail{
		From:     fmt.Sprintf("%s <%s>", es.fromName, es.fromEmail),
		To:       invoice.PayerEmail,
		Subject:  subject,
		HtmlBody: htmlBody,
		TextBody: textBody,
		Tag:      "invoice-reminder",
	}

	return es.sendEmail(email)
}

func (es *EmailService) SendPaymentConfirmationEmail(invoice *models.Invoice, payment *models.Payment) error {
	// Send to payer
	if invoice.PayerEmail != "" {
		if err := es.sendPaymentConfirmationToPayer(invoice, payment); err != nil {
			return fmt.Errorf("failed to send confirmation to payer: %v", err)
		}
	}

	// Send to creator (if we have their email)
	// Note: We could store creator email in the invoice model if needed
	return nil
}

func (es *EmailService) sendPaymentConfirmationToPayer(invoice *models.Invoice, payment *models.Payment) error {
	subject := fmt.Sprintf("Payment Confirmed - Invoice #%d", invoice.InvoiceID)

	htmlBody, textBody, err := es.generatePaymentConfirmationContent(invoice, payment)
	if err != nil {
		return fmt.Errorf("failed to generate confirmation email content: %v", err)
	}

	email := PostmarkEmail{
		From:     fmt.Sprintf("%s <%s>", es.fromName, es.fromEmail),
		To:       invoice.PayerEmail,
		Subject:  subject,
		HtmlBody: htmlBody,
		TextBody: textBody,
		Tag:      "payment-confirmation",
	}

	return es.sendEmail(email)
}

func (es *EmailService) sendEmail(email PostmarkEmail) error {
	if es.apiKey == "" {
		return fmt.Errorf("POSTMARK_API_KEY not configured")
	}

	jsonData, err := json.Marshal(email)
	if err != nil {
		return fmt.Errorf("failed to marshal email data: %v", err)
	}

	req, err := http.NewRequest("POST", es.baseURL+"/email", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Postmark-Server-Token", es.apiKey)

	resp, err := es.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}
	defer resp.Body.Close()

	var postmarkResp PostmarkResponse
	if err := json.NewDecoder(resp.Body).Decode(&postmarkResp); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}

	if postmarkResp.ErrorCode != 0 {
		return fmt.Errorf("postmark error %d: %s", postmarkResp.ErrorCode, postmarkResp.Message)
	}

	return nil
}

func (es *EmailService) generateInvoiceEmailContent(invoice *models.Invoice) (string, string, error) {
	// HTML template
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Invoice #{{.InvoiceID}}</title>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #f8f9fa; padding: 20px; border-radius: 8px; margin-bottom: 20px; }
        .invoice-details { background: #fff; border: 1px solid #dee2e6; border-radius: 8px; padding: 20px; margin-bottom: 20px; }
        .amount { font-size: 24px; font-weight: bold; color: #28a745; }
        .button { display: inline-block; background: #007bff; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; margin: 10px 0; }
        .footer { margin-top: 30px; padding-top: 20px; border-top: 1px solid #dee2e6; font-size: 14px; color: #6c757d; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Invoice #{{.InvoiceID}}</h1>
            {{if .CreatorName}}<p>From: {{.CreatorName}}</p>{{end}}
        </div>
        
        <div class="invoice-details">
            <h2>{{.Title}}</h2>
            {{if .Description}}<p>{{.Description}}</p>{{end}}
            
            <p><strong>Amount Due:</strong> <span class="amount">${{.AmountFormatted}} USDC</span></p>
            {{if .DueDate}}<p><strong>Due Date:</strong> {{.DueDateFormatted}}</p>{{end}}
        </div>
        
        <div style="text-align: center;">
            <a href="{{.PaymentLink}}" class="button">Pay Invoice</a>
            <p>Or scan this QR code with your mobile wallet:</p>
            <img src="{{.QRCodeURL}}" alt="Payment QR Code" style="max-width: 200px;">
        </div>
        
        <div class="footer">
            <p>This invoice is powered by secure blockchain technology. Your payment will be processed automatically.</p>
        </div>
    </div>
</body>
</html>`

	// Text template
	textTemplate := `Invoice #{{.InvoiceID}}
{{if .CreatorName}}From: {{.CreatorName}}{{end}}

{{.Title}}
{{if .Description}}{{.Description}}{{end}}

Amount Due: ${{.AmountFormatted}} USDC
{{if .DueDate}}Due Date: {{.DueDateFormatted}}{{end}}

Pay online: {{.PaymentLink}}

This invoice is powered by secure blockchain technology.`

	data := struct {
		*models.Invoice
		AmountFormatted  string
		DueDateFormatted string
	}{
		Invoice:          invoice,
		AmountFormatted:  formatUSDC(invoice.Amount),
		DueDateFormatted: formatDate(invoice.DueDate),
	}

	htmlTmpl, err := template.New("html").Parse(htmlTemplate)
	if err != nil {
		return "", "", err
	}

	textTmpl, err := template.New("text").Parse(textTemplate)
	if err != nil {
		return "", "", err
	}

	var htmlBuf, textBuf bytes.Buffer

	if err := htmlTmpl.Execute(&htmlBuf, data); err != nil {
		return "", "", err
	}

	if err := textTmpl.Execute(&textBuf, data); err != nil {
		return "", "", err
	}

	return htmlBuf.String(), textBuf.String(), nil
}

func (es *EmailService) generateReminderEmailContent(invoice *models.Invoice) (string, string, error) {
	// Similar to invoice email but with reminder messaging
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Payment Reminder - Invoice #{{.InvoiceID}}</title>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #fff3cd; padding: 20px; border-radius: 8px; margin-bottom: 20px; border: 1px solid #ffeaa7; }
        .invoice-details { background: #fff; border: 1px solid #dee2e6; border-radius: 8px; padding: 20px; margin-bottom: 20px; }
        .amount { font-size: 24px; font-weight: bold; color: #dc3545; }
        .button { display: inline-block; background: #dc3545; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; margin: 10px 0; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>⏰ Payment Reminder</h1>
            <p>Invoice #{{.InvoiceID}} payment is due</p>
        </div>
        
        <div class="invoice-details">
            <h2>{{.Title}}</h2>
            <p><strong>Amount Due:</strong> <span class="amount">${{.AmountFormatted}} USDC</span></p>
            {{if .DueDate}}<p><strong>Due Date:</strong> {{.DueDateFormatted}}</p>{{end}}
        </div>
        
        <div style="text-align: center;">
            <a href="{{.PaymentLink}}" class="button">Pay Now</a>
        </div>
    </div>
</body>
</html>`

	textTemplate := `Payment Reminder - Invoice #{{.InvoiceID}}

{{.Title}}
Amount Due: ${{.AmountFormatted}} USDC
{{if .DueDate}}Due Date: {{.DueDateFormatted}}{{end}}

Pay now: {{.PaymentLink}}`

	data := struct {
		*models.Invoice
		AmountFormatted  string
		DueDateFormatted string
	}{
		Invoice:          invoice,
		AmountFormatted:  formatUSDC(invoice.Amount),
		DueDateFormatted: formatDate(invoice.DueDate),
	}

	htmlTmpl, err := template.New("html").Parse(htmlTemplate)
	if err != nil {
		return "", "", err
	}

	textTmpl, err := template.New("text").Parse(textTemplate)
	if err != nil {
		return "", "", err
	}

	var htmlBuf, textBuf bytes.Buffer

	if err := htmlTmpl.Execute(&htmlBuf, data); err != nil {
		return "", "", err
	}

	if err := textTmpl.Execute(&textBuf, data); err != nil {
		return "", "", err
	}

	return htmlBuf.String(), textBuf.String(), nil
}

func (es *EmailService) generatePaymentConfirmationContent(invoice *models.Invoice, payment *models.Payment) (string, string, error) {
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Payment Confirmed - Invoice #{{.InvoiceID}}</title>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #d4edda; padding: 20px; border-radius: 8px; margin-bottom: 20px; border: 1px solid #c3e6cb; }
        .payment-details { background: #fff; border: 1px solid #dee2e6; border-radius: 8px; padding: 20px; margin-bottom: 20px; }
        .amount { font-size: 24px; font-weight: bold; color: #28a745; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>✅ Payment Confirmed</h1>
            <p>Thank you for your payment!</p>
        </div>
        
        <div class="payment-details">
            <h2>Invoice #{{.InvoiceID}} - {{.Title}}</h2>
            <p><strong>Amount Paid:</strong> <span class="amount">${{.AmountFormatted}} USDC</span></p>
            <p><strong>Transaction:</strong> <a href="https://etherscan.io/tx/{{.TxHash}}" target="_blank">{{.TxHashShort}}</a></p>
        </div>
    </div>
</body>
</html>`

	textTemplate := `Payment Confirmed ✅

Invoice #{{.InvoiceID}} - {{.Title}}
Amount Paid: ${{.AmountFormatted}} USDC
Transaction: https://etherscan.io/tx/{{.TxHash}}

Thank you for your payment!`

	data := struct {
		*models.Invoice
		*models.Payment
		AmountFormatted string
		TxHashShort     string
	}{
		Invoice:         invoice,
		Payment:         payment,
		AmountFormatted: formatUSDC(payment.Amount),
		TxHashShort:     payment.TxHash[:10] + "...",
	}

	htmlTmpl, err := template.New("html").Parse(htmlTemplate)
	if err != nil {
		return "", "", err
	}

	textTmpl, err := template.New("text").Parse(textTemplate)
	if err != nil {
		return "", "", err
	}

	var htmlBuf, textBuf bytes.Buffer

	if err := htmlTmpl.Execute(&htmlBuf, data); err != nil {
		return "", "", err
	}

	if err := textTmpl.Execute(&textBuf, data); err != nil {
		return "", "", err
	}

	return htmlBuf.String(), textBuf.String(), nil
}

// Helper functions
func formatUSDC(amount uint64) string {
	return fmt.Sprintf("%.2f", float64(amount)/1000000) // USDC has 6 decimals
}

func formatDate(date *time.Time) string {
	if date == nil {
		return ""
	}
	return date.Format("January 2, 2006")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
