package server

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/s3"

	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/webp"
)

// contractUploader is indirected so tests can stub S3 writes without
// standing up a real bucket.
var contractUploader = s3.UploadFileProtected

var publicUploader = s3.UploadFile

// publicDeleter is indirected so tests can count or stub public-store deletes.
var publicDeleter = s3.DeleteFile

// maxUploadFolderSegments / uploadFolderSegment bound the caller-chosen
// subfolder under businesses/<id>/ (the frontend sends one slug such as
// "menu-item" or "gallery").
const maxUploadFolderSegments = 3

// reservedUploadFolders are first segments under businesses/<id>/ that public
// uploads must not use: "contracts" is the protected-upload namespace that
// /media refuses to serve.
var reservedUploadFolders = map[string]struct{}{
	"contracts": {},
}

// maxProtectedUploadBytes caps a single protected upload at 10 MiB. The
// server-wide MaxMultipartMemory is 100 MiB which is far too generous
// for contract PDFs / logos and leaves us exposed to resource-
// exhaustion attacks by any authenticated user.
const maxProtectedUploadBytes = 10 << 20

const maxLogoUploadBytes = 5 << 20

// multipartOverheadAllowanceBytes caps total request overhead while still
// allowing a file exactly equal to its policy MaxBytes through multipart
// boundaries, headers, and small form fields.
const multipartOverheadAllowanceBytes = 1 << 20

type uploadPolicy struct {
	MaxBytes    int64
	AllowedMIME map[string]struct{}
}

var imageUploadMIMEs = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/webp": {},
}

var publicBusinessUploadMIMEs = map[string]struct{}{
	"application/pdf": {},
	"image/png":       {},
	"image/jpeg":      {},
	"image/webp":      {},
}

var uploadExtensionsByMIME = map[string]map[string]struct{}{
	"application/pdf": {
		".pdf": {},
	},
	"image/png": {
		".png": {},
	},
	"image/jpeg": {
		".jpg":  {},
		".jpeg": {},
	},
	"image/webp": {
		".webp": {},
	},
}

// allowedProtectedUploadMIMEs is the MIME allowlist for protected business
// uploads. The endpoint is intended for contract PDFs
// and small image attachments. HTML/SVG/JS/executables are rejected
// because they enable stored-XSS and arbitrary code distribution from
// the instance's trusted media domain.
var allowedProtectedUploadMIMEs = map[string]struct{}{
	"application/pdf": {},
	"image/png":       {},
	"image/jpeg":      {},
	"image/webp":      {},
}

// UploadFile handles file uploads to S3
func UploadFile(c *gin.Context) {
	business, ok := requireFileBusinessAccess(c, "public_upload")
	if !ok {
		return
	}

	if !parseMultipartWithUploadCap(c, maxProtectedUploadBytes) {
		return
	}

	// Get the file from form data
	file, err := c.FormFile("file")
	if err != nil {
		if isMaxBytesError(err) {
			respondPayloadTooLarge(c, maxProtectedUploadBytes)
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}

	safeName, contentType, ok := validateUploadedFile(c, file, uploadPolicy{
		MaxBytes:    maxProtectedUploadBytes,
		AllowedMIME: publicBusinessUploadMIMEs,
	})
	if !ok {
		return
	}

	// Generate random filename to prevent conflicts
	fileName := generateUniqueFilename(safeName)

	// Create folder path with business ID
	folderPath := fmt.Sprintf("businesses/%d", business.ID)

	// Allow a custom subfolder within the business folder. Every segment must
	// be a plain slug, so a caller can't escape into another tenant's prefix
	// ("..", which S3 SDKs normalize away), smuggle encoded separators, or
	// write into a reserved namespace.
	if raw := c.PostForm("folder"); raw != "" {
		cleaned, err := cleanUploadFolder(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid folder path"})
			return
		}
		if cleaned != "" {
			folderPath = folderPath + "/" + cleaned
		}
	}

	// Upload file to S3
	location, err := publicUploader(
		file,
		fileName,
		folderPath,
		s3.WithContentType(contentType),
		s3.WithContentDisposition(inlineContentDisposition(fileName)),
	)
	if err != nil {
		metrics.FileMutationOutcomes.WithLabelValues("public_upload", "s3_failure").Inc()
		log.Printf("[UploadFile] S3 upload failed (business=%d): %v", business.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload file"})
		return
	}
	metrics.FileMutationOutcomes.WithLabelValues("public_upload", "success").Inc()

	c.JSON(http.StatusOK, gin.H{
		"location":    location,
		"filename":    fileName,
		"folder":      folderPath,
		"business_id": business.ID,
	})
}

// UploadBusinessLogo handles a tenant-scoped business logo upload.
func UploadBusinessLogo(c *gin.Context) {
	business, ok := requireFileBusinessAccess(c, "logo_upload")
	if !ok {
		return
	}

	if !parseMultipartWithUploadCap(c, maxLogoUploadBytes) {
		return
	}

	// Get the file from form data
	file, err := c.FormFile("file")
	if err != nil {
		if isMaxBytesError(err) {
			respondPayloadTooLarge(c, maxLogoUploadBytes)
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}

	safeName, contentType, ok := validateUploadedFile(c, file, uploadPolicy{
		MaxBytes:    maxLogoUploadBytes,
		AllowedMIME: imageUploadMIMEs,
	})
	if !ok {
		return
	}

	// Generate random filename to prevent conflicts
	fileName := generateUniqueFilename(safeName)
	folderPath := fmt.Sprintf("businesses/%d/logo", business.ID)

	// Upload file to S3
	location, err := publicUploader(
		file,
		fileName,
		folderPath,
		s3.WithContentType(contentType),
		s3.WithContentDisposition(inlineContentDisposition(fileName)),
	)
	if err != nil {
		metrics.FileMutationOutcomes.WithLabelValues("logo_upload", "s3_failure").Inc()
		log.Printf("[UploadBusinessLogo] S3 upload failed (business=%d): %v", business.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload file"})
		return
	}
	metrics.FileMutationOutcomes.WithLabelValues("logo_upload", "success").Inc()

	c.JSON(http.StatusOK, gin.H{
		"location":    location,
		"filename":    fileName,
		"folder":      folderPath,
		"business_id": business.ID,
	})
}

// UploadFileProtected handles authenticated, tenant-scoped uploads to
// the protected S3 bucket.
//
// Security:
//   - The caller MUST be authorized for the URL-scoped business; the key
//     is derived server-side as `businesses/<id>/contracts/<filename>`.
//     Any caller-supplied `folder` is ignored to block path traversal
//     like `../../businesses/<other>/`.
//   - Content is sniffed via http.DetectContentType and must be one of
//     {application/pdf, image/png, image/jpeg, image/webp}. HTML, SVG,
//     JS, and executables are rejected to prevent stored-XSS and
//     arbitrary code distribution.
//   - The body is capped at 10 MiB via http.MaxBytesReader BEFORE
//     multipart parsing so a malicious client cannot exhaust memory by
//     sending a multi-GiB form.
func UploadFileProtected(c *gin.Context) {
	business, ok := requireFileBusinessAccess(c, "protected_upload")
	if !ok {
		return
	}
	businessID := business.ID

	if !parseMultipartWithUploadCap(c, maxProtectedUploadBytes) {
		return
	}

	// Retrieve the uploaded file from the already-parsed form.
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if isMaxBytesError(err) {
			respondPayloadTooLarge(c, maxProtectedUploadBytes)
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}

	// Derive the S3 key server-side — never trust caller-supplied folder.
	safeName, contentType, ok := validateUploadedFile(c, fileHeader, uploadPolicy{
		MaxBytes:    maxProtectedUploadBytes,
		AllowedMIME: allowedProtectedUploadMIMEs,
	})
	if !ok {
		return
	}
	folderPath := fmt.Sprintf("businesses/%d/contracts", businessID)
	// A unique stored name: two uploads of "contract.pdf" must never overwrite
	// each other (the protected store has no versioning to recover from).
	storedName := generateUniqueFilename(safeName)

	location, err := contractUploader(
		fileHeader,
		storedName,
		folderPath,
		s3.WithContentType(contentType),
	)
	if err != nil {
		metrics.FileMutationOutcomes.WithLabelValues("protected_upload", "s3_failure").Inc()
		log.Printf("[UploadFileProtected] S3 upload failed for business %d: %v", businessID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload file"})
		return
	}
	metrics.FileMutationOutcomes.WithLabelValues("protected_upload", "success").Inc()

	c.JSON(http.StatusOK, gin.H{
		"location":   location,
		"filename":   safeName,
		"folder":     folderPath,
		"businessId": businessID,
	})
}

func validateUploadedFile(c *gin.Context, fileHeader *multipart.FileHeader, policy uploadPolicy) (safeName string, sniffed string, ok bool) {
	if fileHeader.Size > policy.MaxBytes {
		respondPayloadTooLarge(c, policy.MaxBytes)
		return "", "", false
	}

	rawName, err := rawMultipartFilename(fileHeader)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid filename"})
		return "", "", false
	}

	safeName = filepath.Base(fileHeader.Filename)
	if rawName == "" ||
		rawName == "." ||
		rawName == "/" ||
		strings.ContainsAny(rawName, "/\\") ||
		fileHeader.Filename == "" ||
		fileHeader.Filename == "." ||
		fileHeader.Filename == "/" ||
		safeName == "." ||
		safeName == "/" ||
		safeName == "" ||
		strings.ContainsAny(fileHeader.Filename, "/\\") ||
		strings.ContainsAny(safeName, "/\\") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid filename"})
		return "", "", false
	}

	sniffed, err = sniffUploadedContentType(fileHeader)
	if err != nil {
		if isMaxBytesError(err) {
			respondPayloadTooLarge(c, policy.MaxBytes)
			return "", "", false
		}
		log.Printf("[UploadFile] Failed to read uploaded file: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read uploaded file"})
		return "", "", false
	}
	if _, allowed := policy.AllowedMIME[sniffed]; !allowed {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"error": fmt.Sprintf("unsupported content type %q; allowed: %s", sniffed, formatAllowedMIMEs(policy.AllowedMIME)),
			"code":  "unsupported_media_type",
		})
		return "", "", false
	}
	if !extensionAllowedForMIME(safeName, sniffed) {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"error": fmt.Sprintf("file extension does not match content type %q", sniffed),
			"code":  "unsupported_media_type",
		})
		return "", "", false
	}
	if err := validateUploadedImageContent(fileHeader, sniffed, policy.MaxBytes); err != nil {
		if isMaxBytesError(err) {
			respondPayloadTooLarge(c, policy.MaxBytes)
			return "", "", false
		}
		log.Printf("[UploadFile] Rejected undecodable %s upload: %v", sniffed, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("uploaded file is not a valid %s image", sniffed),
			"code":  "invalid_image",
		})
		return "", "", false
	}

	return safeName, sniffed, true
}

// Content sniffing only looks at magic bytes, so three bytes of JPEG prefix
// used to be enough to get a file published to the public image host (#929).
// Uploads that claim to be images must actually decode.
const (
	// #929 asked for undecodable bytes to be refused, not for a size policy,
	// and every upload surface accepts image/*. The floor is therefore "has
	// pixels at all" -- anything stricter refuses artwork that uploaded fine
	// before this check existed (favicon-class icons, 1x1 spacers). Garbage is
	// caught by having to decode, not by being small.
	minUploadImageSide = 1
	// Reject decompression bombs from their header, before image.Decode
	// allocates the pixel buffer they advertise. Mirrors the limits the
	// generated-image delivery check uses in internal/services.
	maxUploadImageSide      = 8192
	maxUploadImagePixelArea = int64(32 << 20) // 32 megapixels
)

// decodedUploadImageMIME maps an image/* decoder format name back to the MIME
// type the sniffer produced, so bytes that decode as a different format than
// they advertise are rejected.
func decodedUploadImageMIME(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		return "image/png"
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

// validateUploadedImageContent decodes an uploaded image and enforces sane
// dimensions. Non-image uploads (contract PDFs) pass straight through.
func validateUploadedImageContent(fh *multipart.FileHeader, sniffed string, maxBytes int64) error {
	if _, ok := imageUploadMIMEs[sniffed]; !ok {
		return nil
	}

	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	// Bounded by the same cap the policy already enforced on fileHeader.Size.
	data, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return err
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode image configuration: %w", err)
	}
	if config.Width < minUploadImageSide || config.Height < minUploadImageSide {
		return fmt.Errorf("image is %dx%d; minimum side is %d", config.Width, config.Height, minUploadImageSide)
	}
	if config.Width > maxUploadImageSide || config.Height > maxUploadImageSide {
		return fmt.Errorf("image is %dx%d; maximum side is %d", config.Width, config.Height, maxUploadImageSide)
	}
	if pixels := int64(config.Width) * int64(config.Height); pixels > maxUploadImagePixelArea {
		return fmt.Errorf("image pixel area %d exceeds maximum %d", pixels, maxUploadImagePixelArea)
	}
	if decoded := decodedUploadImageMIME(format); decoded != sniffed {
		return fmt.Errorf("decoded format %q does not match sniffed type %q", format, sniffed)
	}

	// An animated WebP cannot survive image.Decode, so it is verified
	// structurally instead. The dimension caps above already ran, which is what
	// bounds the work either path can be made to do.
	if sniffed == "image/webp" {
		animated, err := validateAnimatedWebP(data)
		if err != nil {
			return err
		}
		if animated {
			return nil
		}
	}

	// DecodeConfig only reads the header: a truncated or corrupt body still has
	// to fail before the bytes reach S3.
	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != config.Width || bounds.Dy() != config.Height {
		return fmt.Errorf("decoded dimensions %dx%d do not match header %dx%d",
			bounds.Dx(), bounds.Dy(), config.Width, config.Height)
	}
	if decodedMIME := decodedUploadImageMIME(format); decodedMIME != sniffed {
		return fmt.Errorf("fully decoded format %q does not match sniffed type %q", format, sniffed)
	}
	return nil
}

// RIFF/WebP container constants used to tell an animation apart from a still.
const (
	webpAnimationFlag      = 1 << 1 // VP8X feature byte
	webpVP8XPayloadBytes   = 10
	webpANMFHeaderBytes    = 16
	webpChunkHeaderBytes   = 8
	webpRIFFHeaderBytes    = 12
	webpMinContainerLength = webpRIFFHeaderBytes
)

// validateAnimatedWebP reports whether data is an animated WebP and, when it
// is, that the animation actually carries decodable frames.
//
// golang.org/x/image/webp answers DecodeConfig straight off the VP8X chunk but
// can only fully decode a still: an animation stores its frames inside ANMF
// chunks, so image.Decode walks the container, never finds a top-level
// "VP8 "/"VP8L" chunk, and reports "webp: invalid format". Treating that as
// invalid would reject files that upload fine on every surface -- every picker
// sends accept="image/*", and any modern GIF-to-WebP export lands here -- so
// animations are checked structurally instead. That is still a decode of the
// container: a VP8X header with the animation bit and nothing behind it does
// not pass.
//
// A false return means "not an animation as far as this parser can tell" and
// leaves the verdict to image.Decode, so malformed stills keep failing there.
func validateAnimatedWebP(data []byte) (bool, error) {
	if len(data) < webpMinContainerLength ||
		string(data[0:4]) != "RIFF" ||
		string(data[8:12]) != "WEBP" {
		return false, nil
	}
	end := int64(webpChunkHeaderBytes) + int64(binary.LittleEndian.Uint32(data[4:8]))
	if end < webpRIFFHeaderBytes || end > int64(len(data)) {
		return false, nil
	}

	animated := false
	frames := 0
	for off := int64(webpRIFFHeaderBytes); off+webpChunkHeaderBytes <= end; {
		id := string(data[off : off+4])
		size := int64(binary.LittleEndian.Uint32(data[off+4 : off+webpChunkHeaderBytes]))
		if off+webpChunkHeaderBytes+size > end {
			if animated {
				return true, fmt.Errorf("animated webp chunk %q runs past its container", id)
			}
			return false, nil
		}
		payload := data[off+webpChunkHeaderBytes : off+webpChunkHeaderBytes+size]

		switch id {
		case "VP8X":
			// VP8X is required to be the first chunk, and it is the only place
			// the animation bit lives.
			if off != webpRIFFHeaderBytes || size != webpVP8XPayloadBytes {
				return false, nil
			}
			if payload[0]&webpAnimationFlag == 0 {
				return false, nil // a still image; image.Decode judges it
			}
			animated = true
		case "ANMF":
			if !animated {
				return false, nil
			}
			if !webpFrameCarriesImage(payload) {
				return true, errors.New("animated webp frame carries no image data")
			}
			frames++
		}

		off += webpChunkHeaderBytes + size + size%2
	}

	if !animated {
		return false, nil
	}
	if frames == 0 {
		return true, errors.New("animated webp carries no frames")
	}
	return true, nil
}

// webpFrameCarriesImage checks that an ANMF payload holds its 16-byte frame
// header followed by a real image sub-chunk: "VP8 " or "VP8L", optionally
// preceded by an ALPH alpha plane.
func webpFrameCarriesImage(payload []byte) bool {
	for off := int64(webpANMFHeaderBytes); off+webpChunkHeaderBytes <= int64(len(payload)); {
		id := string(payload[off : off+4])
		size := int64(binary.LittleEndian.Uint32(payload[off+4 : off+webpChunkHeaderBytes]))
		if size <= 0 || off+webpChunkHeaderBytes+size > int64(len(payload)) {
			return false
		}
		if id == "VP8 " || id == "VP8L" {
			return true
		}
		if id != "ALPH" {
			return false
		}
		off += webpChunkHeaderBytes + size + size%2
	}
	return false
}

func extensionAllowedForMIME(name, contentType string) bool {
	allowed, ok := uploadExtensionsByMIME[contentType]
	if !ok {
		return false
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return false
	}
	_, ok = allowed[ext]
	return ok
}

func inlineContentDisposition(filename string) string {
	return mime.FormatMediaType("inline", map[string]string{"filename": filename})
}

func parseMultipartWithUploadCap(c *gin.Context, maxFileBytes int64) bool {
	bodyCap := maxFileBytes + multipartOverheadAllowanceBytes
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, bodyCap)
	if err := c.Request.ParseMultipartForm(bodyCap); err != nil {
		if isMaxBytesError(err) {
			respondPayloadTooLarge(c, maxFileBytes)
			return false
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart form"})
		return false
	}
	return true
}

func rawMultipartFilename(fileHeader *multipart.FileHeader) (string, error) {
	disposition := fileHeader.Header.Get("Content-Disposition")
	if disposition == "" {
		return fileHeader.Filename, nil
	}

	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return "", err
	}
	if filename, ok := params["filename"]; ok {
		return filename, nil
	}
	return fileHeader.Filename, nil
}

func formatAllowedMIMEs(allowed map[string]struct{}) string {
	values := make([]string, 0, len(allowed))
	for value := range allowed {
		values = append(values, value)
	}
	return strings.Join(values, ", ")
}

// sniffUploadedContentType opens the multipart file, reads up to 512
// bytes, and runs http.DetectContentType on them. This is the standard
// net/http content sniff and matches how browsers decide whether a blob
// is HTML/JS/etc.
func sniffUploadedContentType(fh *multipart.FileHeader) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	ct := http.DetectContentType(head[:n])
	// DetectContentType appends "; charset=..." for text types; strip it
	// so the allowlist comparison is straightforward.
	if semi := strings.Index(ct, ";"); semi >= 0 {
		ct = strings.TrimSpace(ct[:semi])
	}
	return ct, nil
}

// isMaxBytesError reports whether err was produced by
// http.MaxBytesReader exceeding its cap. Works across stdlib versions
// that pre- and post-date http.MaxBytesError.
func isMaxBytesError(err error) bool {
	if err == nil {
		return false
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "http: request body too large") ||
		strings.Contains(msg, "multipart: Part size limit exceeded") ||
		strings.Contains(msg, "multipart: message too large")
}

func respondPayloadTooLarge(c *gin.Context, maxBytes int64) {
	c.JSON(http.StatusRequestEntityTooLarge, gin.H{
		"error": fmt.Sprintf("upload exceeds %d bytes", maxBytes),
		"code":  "payload_too_large",
	})
}

// DeleteUploadedFile deletes a previously uploaded public business file.
// The key may be a bare key, a /media URL, a CDN URL or a legacy S3 URL; it
// must resolve inside businesses/<id>/ of the authorized business.
func DeleteUploadedFile(c *gin.Context) {
	business, ok := requireFileBusinessAccess(c, "delete")
	if !ok {
		return
	}

	key, err := storageKeyFromInput(strings.TrimSpace(c.Query("key")))
	if err != nil {
		metrics.FileMutationOutcomes.WithLabelValues("delete", "invalid_key").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !keyWithinBusinessUploads(key, business.ID) {
		metrics.FileMutationOutcomes.WithLabelValues("delete", "tenant_mismatch").Inc()
		c.JSON(http.StatusForbidden, gin.H{"error": "File not found or you don't have permission"})
		return
	}

	if err := publicDeleter(key); err != nil {
		metrics.FileMutationOutcomes.WithLabelValues("delete", "s3_failure").Inc()
		log.Printf("[DeleteUploadedFile] Failed to delete %q: %v", key, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file"})
		return
	}
	metrics.FileMutationOutcomes.WithLabelValues("delete", "success").Inc()

	c.JSON(http.StatusOK, gin.H{"message": "File deleted successfully"})
}

var errInvalidStorageKey = errors.New("invalid key")

// storageKeyFromInput resolves a caller-supplied key or URL to an object key
// (s3.KeyFromURL understands /media, CDN and legacy S3 URLs and decodes
// percent-escapes), then re-checks it is already clean: no ".", ".." or empty
// segments, no absolute path.
func storageKeyFromInput(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("key is required")
	}
	key, err := s3.KeyFromURL(raw)
	if err != nil {
		return "", errInvalidStorageKey
	}
	if path.Clean(key) != key || strings.HasPrefix(key, "/") {
		return "", errInvalidStorageKey
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", errInvalidStorageKey
		}
	}
	return key, nil
}

// keyWithinBusinessUploads reports whether key names an object under
// businesses/<id>/ outside the reserved (protected) namespaces.
func keyWithinBusinessUploads(key string, businessID uint) bool {
	rest, ok := strings.CutPrefix(key, fmt.Sprintf("businesses/%d/", businessID))
	// IsProtectedMediaKey covers businesses/<id>/contracts/ (protected uploads).
	return ok && rest != "" && !s3.IsProtectedMediaKey(key)
}

// cleanUploadFolder validates a caller-chosen upload subfolder: 1-3 slug
// segments ([A-Za-z0-9_-], starting alphanumeric, <=64 chars each), outer
// slashes trimmed, not starting with a reserved namespace. Returns "" for an
// all-slash input.
func cleanUploadFolder(raw string) (string, error) {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", nil
	}
	segments := strings.Split(trimmed, "/")
	if len(segments) > maxUploadFolderSegments {
		return "", errors.New("folder too deep")
	}
	for _, seg := range segments {
		if !isUploadFolderSegment(seg) {
			return "", errors.New("invalid folder segment")
		}
	}
	if _, reserved := reservedUploadFolders[strings.ToLower(segments[0])]; reserved {
		return "", errors.New("reserved folder")
	}
	return trimmed, nil
}

func isUploadFolderSegment(seg string) bool {
	if seg == "" || len(seg) > 64 {
		return false
	}
	for i, r := range seg {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case (r == '-' || r == '_') && i > 0:
		default:
			return false
		}
	}
	return true
}

func requireFileBusinessAccess(c *gin.Context, operation string) (*database.Business, bool) {
	if strings.TrimSpace(c.Param("id")) == "" {
		metrics.FileMutationOutcomes.WithLabelValues(operation, "missing_business_id").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Business id is required"})
		c.Abort()
		return nil, false
	}
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		metrics.FileMutationOutcomes.WithLabelValues(operation, "authorization_denied").Inc()
		return nil, false
	}
	return business, true
}

// generateUniqueFilename creates a unique filename with random string and timestamp
func generateUniqueFilename(originalFilename string) string {
	ext := filepath.Ext(originalFilename)

	// Generate random bytes for unique identifier
	randomBytes := make([]byte, 8) // 16 character hex string
	if _, err := rand.Read(randomBytes); err != nil {
		log.Printf("WARNING: failed to generate random bytes for filename: %v", err)
	}
	randomString := hex.EncodeToString(randomBytes)

	// Create timestamp
	timestamp := time.Now().Format("20060102_150405")

	// Create filename: random_timestamp.ext
	return fmt.Sprintf("%s_%s%s", randomString, timestamp, ext)
}
