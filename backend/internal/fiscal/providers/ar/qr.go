package ar

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

const qrBaseURL = "https://www.arca.gob.ar/fe/qr/"

type QRInput struct {
	Date          time.Time
	CUIT          int64
	PointOfSale   int
	ReceiptType   int
	ReceiptNumber int64
	Amount        float64
	Currency      string
	ExchangeRate  float64
	DocType       int
	DocNumber     string
	AuthType      string
	AuthCode      int64
}

func BuildQRURL(input QRInput) (string, error) {
	docNumber, err := qrDocNumberPayload(input.DocNumber)
	if err != nil {
		return "", err
	}
	payload := map[string]interface{}{
		"ver":        1,
		"fecha":      input.Date.In(argentinaLocation).Format("2006-01-02"),
		"cuit":       input.CUIT,
		"ptoVta":     input.PointOfSale,
		"tipoCmp":    input.ReceiptType,
		"nroCmp":     input.ReceiptNumber,
		"importe":    input.Amount,
		"moneda":     input.Currency,
		"ctz":        input.ExchangeRate,
		"tipoDocRec": input.DocType,
		"nroDocRec":  docNumber,
		"tipoCodAut": input.AuthType,
		"codAut":     input.AuthCode,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(qrBaseURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("p", base64.StdEncoding.EncodeToString(raw))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func qrDocNumberPayload(docNumber string) (json.RawMessage, error) {
	if docNumber == "" {
		return json.RawMessage("0"), nil
	}
	if len(docNumber) > 20 {
		return nil, fmt.Errorf("doc number must be at most 20 digits")
	}
	for _, r := range docNumber {
		if r < '0' || r > '9' {
			return nil, fmt.Errorf("doc number must contain only digits")
		}
	}
	return json.RawMessage(docNumber), nil
}
