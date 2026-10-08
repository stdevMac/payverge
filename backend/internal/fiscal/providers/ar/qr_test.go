package ar

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildQRURL(t *testing.T) {
	u, err := BuildQRURL(QRInput{
		// 15:00 UTC is 12:00 in Argentina, so the civil date stays 21 May.
		Date:          time.Date(2026, 5, 21, 15, 0, 0, 0, time.UTC),
		CUIT:          30000000007,
		PointOfSale:   1,
		ReceiptType:   6,
		ReceiptNumber: 1234,
		Amount:        1234.56,
		Currency:      "PES",
		ExchangeRate:  1,
		DocType:       96,
		DocNumber:     "12345678",
		AuthType:      "E",
		AuthCode:      70123456789012,
	})
	require.NoError(t, err)
	parsed, err := url.Parse(u)
	require.NoError(t, err)
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "www.arca.gob.ar", parsed.Host)
	require.Equal(t, "/fe/qr/", parsed.Path)
	payload := parsed.Query().Get("p")
	require.NotEmpty(t, payload)
	raw, err := base64.StdEncoding.DecodeString(payload)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, float64(1), decoded["ver"])
	require.Equal(t, "2026-05-21", decoded["fecha"])
	require.Equal(t, float64(30000000007), decoded["cuit"])
	require.Equal(t, float64(1), decoded["ptoVta"])
	require.Equal(t, float64(6), decoded["tipoCmp"])
	require.Equal(t, float64(1234), decoded["nroCmp"])
	require.Equal(t, float64(1234.56), decoded["importe"])
	require.Equal(t, "PES", decoded["moneda"])
	require.Equal(t, float64(1), decoded["ctz"])
	require.Equal(t, float64(96), decoded["tipoDocRec"])
	require.Equal(t, float64(12345678), decoded["nroDocRec"])
	require.Equal(t, "E", decoded["tipoCodAut"])
	require.Equal(t, float64(70123456789012), decoded["codAut"])
}

func TestBuildQRURL_FechaUsesArgentinaCivilDate(t *testing.T) {
	// 02:30 UTC on 10 Mar is 23:30 on 9 Mar in Argentina (UTC-3).
	u, err := BuildQRURL(QRInput{
		Date:          time.Date(2026, 3, 10, 2, 30, 0, 0, time.UTC),
		CUIT:          30000000007,
		PointOfSale:   1,
		ReceiptType:   6,
		ReceiptNumber: 1234,
		Amount:        125.50,
		Currency:      "PES",
		ExchangeRate:  1,
		DocType:       99,
		DocNumber:     "",
		AuthType:      "E",
		AuthCode:      70123456789012,
	})
	require.NoError(t, err)
	parsed, err := url.Parse(u)
	require.NoError(t, err)
	raw, err := base64.StdEncoding.DecodeString(parsed.Query().Get("p"))
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, "2026-03-09", decoded["fecha"])
}

func TestBuildQRURLPreservesTwentyDigitDocNumber(t *testing.T) {
	u, err := BuildQRURL(QRInput{
		Date:          time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC),
		CUIT:          30000000007,
		PointOfSale:   1,
		ReceiptType:   6,
		ReceiptNumber: 1234,
		Amount:        1234.56,
		Currency:      "PES",
		ExchangeRate:  1,
		DocType:       80,
		DocNumber:     "12345678901234567890",
		AuthType:      "E",
		AuthCode:      70123456789012,
	})
	require.NoError(t, err)

	parsed, err := url.Parse(u)
	require.NoError(t, err)
	payload := parsed.Query().Get("p")
	require.NotEmpty(t, payload)

	raw, err := base64.StdEncoding.DecodeString(payload)
	require.NoError(t, err)

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded map[string]interface{}
	require.NoError(t, decoder.Decode(&decoded))
	require.Equal(t, json.Number("12345678901234567890"), decoded["nroDocRec"])
	require.Equal(t, "12345678901234567890", decoded["nroDocRec"].(json.Number).String())
}

func TestBuildQRURLRejectsInvalidDocNumber(t *testing.T) {
	_, err := BuildQRURL(QRInput{
		Date:          time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC),
		CUIT:          30000000007,
		PointOfSale:   1,
		ReceiptType:   6,
		ReceiptNumber: 1234,
		Amount:        1234.56,
		Currency:      "PES",
		ExchangeRate:  1,
		DocType:       96,
		DocNumber:     "12A45678",
		AuthType:      "E",
		AuthCode:      70123456789012,
	})
	require.Error(t, err)
}

func TestBuildQRURLRejectsDocNumberLongerThanTwentyDigits(t *testing.T) {
	_, err := BuildQRURL(QRInput{
		Date:          time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC),
		CUIT:          30000000007,
		PointOfSale:   1,
		ReceiptType:   6,
		ReceiptNumber: 1234,
		Amount:        1234.56,
		Currency:      "PES",
		ExchangeRate:  1,
		DocType:       96,
		DocNumber:     "123456789012345678901",
		AuthType:      "E",
		AuthCode:      70123456789012,
	})
	require.Error(t, err)
}
