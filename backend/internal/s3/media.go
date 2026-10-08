package s3

import (
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Cache policies for /media responses.
const (
	// MediaCacheImmutable is sent for content-addressed keys (a 16+ hex-char
	// run or a UUID in the file name): the bytes behind such a key never
	// change, uploads always mint a fresh key.
	MediaCacheImmutable = "public, max-age=31536000, immutable"
	// MediaCacheDefault is sent for stable, human-named keys (demo seed
	// assets) that could in principle be rewritten in place.
	MediaCacheDefault = "public, max-age=3600"
)

// mediaSandboxCSP neuters any active content that slips through: the response
// is rendered in an opaque-origin sandbox with no script, so even a mislabeled
// upload cannot run on the app's origin.
const mediaSandboxCSP = "default-src 'none'; img-src 'self' data:; media-src 'self'; style-src 'unsafe-inline'; sandbox"

// mediaInlineTypes are the only content types /media renders inline. Anything
// else (HTML, SVG, JS, unknown) is served as an application/octet-stream
// attachment, so a same-origin upload can never become a stored-XSS page.
var mediaInlineTypes = map[string]bool{
	"image/png":       true,
	"image/jpeg":      true,
	"image/webp":      true,
	"image/gif":       true,
	"image/avif":      true,
	"application/pdf": true,
	"video/mp4":       true,
	"video/webm":      true,
}

// mediaExtensionTypes is consulted only when an object has no stored type.
var mediaExtensionTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
	".avif": "image/avif",
	".pdf":  "application/pdf",
	".mp4":  "video/mp4",
	".webm": "video/webm",
}

// mediaDeniedPrefixes are key spaces that only ever hold protected objects.
// The public store never receives them (protected uploads go to a separate
// store), so this is defense in depth against a shared or misconfigured
// bucket: /media answers 404 exactly as for a missing object.
var mediaDeniedPrefixes = []string{
	SharedBucketProtectedPrefix,
	"storage-probe/",
	"fiscal-receipts/",
	"ledger/",
	"ai/menu-extraction/",
	"space-scans/",
}

var contentAddressedName = regexp.MustCompile(`(?i)[0-9a-f]{16}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// IsProtectedMediaKey reports whether key belongs to a protected key space
// that /media must never serve.
func IsProtectedMediaKey(key string) bool {
	for _, prefix := range mediaDeniedPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	// businesses/<id>/contracts/... (UploadFileProtected's folder).
	parts := strings.SplitN(key, "/", 4)
	return len(parts) >= 3 && parts[0] == "businesses" && parts[2] == "contracts"
}

// MediaCacheControl returns the Cache-Control value for key.
func MediaCacheControl(key string) string {
	if contentAddressedName.MatchString(path.Base(key)) {
		return MediaCacheImmutable
	}
	return MediaCacheDefault
}

// MediaHandler serves GET/HEAD /media/*key from the public store. Register it
// with the wildcard parameter named "key".
func MediaHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ServeMedia(c.Writer, c.Request, strings.TrimPrefix(c.Param("key"), "/"))
	}
}

// ServeMedia writes the public object key to w. It never lists directories,
// never serves protected key spaces, and answers 404 for every key it will
// not serve so probing reveals nothing.
func ServeMedia(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if ValidateKey(key) != nil || IsProtectedMediaKey(key) {
		mediaNotFound(w)
		return
	}
	store := PublicStore()
	if store == nil {
		http.Error(w, "storage unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()

	if r.Method == http.MethodHead || r.Header.Get("If-None-Match") != "" {
		info, err := store.Stat(ctx, key)
		if err != nil {
			mediaError(w, key, err)
			return
		}
		writeMediaHeaders(w, info)
		if info.ETag != "" && etagMatches(r.Header.Get("If-None-Match"), info.ETag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	obj, err := store.Get(ctx, key)
	if err != nil {
		mediaError(w, key, err)
		return
	}
	defer func() { _ = obj.Body.Close() }()
	writeMediaHeaders(w, &obj.ObjectInfo)
	if rs, ok := obj.Body.(io.ReadSeeker); ok {
		// Range, If-Modified-Since and If-None-Match handling for free.
		http.ServeContent(w, r, "", obj.ModTime, rs)
		return
	}
	if obj.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, obj.Body)
}

func writeMediaHeaders(w http.ResponseWriter, info *ObjectInfo) {
	h := w.Header()
	contentType, inline := mediaContentType(info)
	h.Set("Content-Type", contentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cross-Origin-Resource-Policy", "cross-origin")
	h.Set("Cache-Control", MediaCacheControl(info.Key))
	if info.ETag != "" {
		h.Set("ETag", info.ETag)
	}
	if !info.ModTime.IsZero() {
		h.Set("Last-Modified", info.ModTime.UTC().Format(http.TimeFormat))
	}
	if contentType == "application/pdf" {
		// Browser PDF viewers do not render under a CSP sandbox; allow our
		// own pages to frame menus/receipts, nobody else.
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Content-Security-Policy", "frame-ancestors 'self'")
	} else {
		h.Set("Content-Security-Policy", mediaSandboxCSP)
	}
	if inline {
		if disposition := safeDisposition(info.ContentDisposition); disposition != "" {
			h.Set("Content-Disposition", disposition)
		}
	} else {
		h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(info.Key)}))
	}
}

func mediaContentType(info *ObjectInfo) (string, bool) {
	contentType := normalizeContentType(info.ContentType)
	if contentType == "image/jpg" || contentType == "image/pjpeg" {
		contentType = "image/jpeg"
	}
	if contentType == "" {
		contentType = mediaExtensionTypes[strings.ToLower(path.Ext(info.Key))]
	}
	if mediaInlineTypes[contentType] {
		return contentType, true
	}
	return "application/octet-stream", false
}

// safeDisposition re-serializes a stored Content-Disposition so nothing but a
// well-formed inline/attachment value with a filename reaches the wire.
func safeDisposition(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	disposition, params, err := mime.ParseMediaType(raw)
	if err != nil || (disposition != "inline" && disposition != "attachment") {
		return ""
	}
	clean := map[string]string{}
	if name := params["filename"]; name != "" {
		clean["filename"] = path.Base(name)
	}
	return mime.FormatMediaType(disposition, clean)
}

func etagMatches(ifNoneMatch, etag string) bool {
	want := strings.TrimPrefix(etag, "W/")
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == want {
			return true
		}
	}
	return false
}

func mediaNotFound(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "not found", http.StatusNotFound)
}

func mediaError(w http.ResponseWriter, key string, err error) {
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidKey) {
		mediaNotFound(w)
		return
	}
	log.Printf("media: read %q failed: %v", key, err)
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "storage error", http.StatusBadGateway)
}
