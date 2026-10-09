package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDetectAttachmentContentType_NeverTrustsExtension: content type must come
// from the bytes. The pre-fix code fell back to the FILENAME extension when
// sniffing failed, letting e.g. an HTML payload named evil.pdf be stored and
// served as application/pdf.
func TestDetectAttachmentContentType_NeverTrustsExtension(t *testing.T) {
	// HTML bytes — sniffed text/html, must be rejected no matter the filename.
	_, ok := detectAttachmentContentType([]byte("<html><script>alert(1)</script></html>"))
	require.False(t, ok, "HTML bytes must never be accepted")

	// Random binary garbage — rejected.
	_, ok = detectAttachmentContentType([]byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})
	require.False(t, ok)

	// %PDF magic → application/pdf.
	ct, ok := detectAttachmentContentType([]byte("%PDF-1.7 test content"))
	require.True(t, ok)
	require.Equal(t, "application/pdf", ct)

	// PNG magic → image/png.
	png := append([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 16)...)
	ct, ok = detectAttachmentContentType(png)
	require.True(t, ok)
	require.Equal(t, "image/png", ct)
}

// TestIsHEICBytes: stdlib DetectContentType cannot sniff HEIC (golang/go#52144),
// so HEIC acceptance rides on an explicit ISO-BMFF ftyp/brand check.
func TestIsHEICBytes(t *testing.T) {
	heic := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c', 0, 0, 0, 0}
	require.True(t, isHEICBytes(heic))
	ct, ok := detectAttachmentContentType(heic)
	require.True(t, ok)
	require.Equal(t, "image/heic", ct)

	mif1 := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'm', 'i', 'f', '1', 0, 0, 0, 0}
	require.True(t, isHEICBytes(mif1))

	// MP4 is ISO-BMFF too but not a HEIF brand — reject.
	mp4 := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0}
	require.False(t, isHEICBytes(mp4))

	require.False(t, isHEICBytes([]byte("ftypheic"))) // too short / wrong offset
	require.False(t, isHEICBytes(nil))
}
