package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// IdempotencyKey is the row model for the idempotency_keys table.
// Lives in this package because it's only consumed by the middleware.
type IdempotencyKey struct {
	ID             uint   `gorm:"primaryKey"`
	Key            string `gorm:"not null"`
	Endpoint       string `gorm:"not null"`
	BusinessID     *uint  `gorm:"index"`
	RequestHash    string `gorm:"not null"`
	ResponseStatus int    `gorm:"not null;default:0"`
	ResponseBody   []byte
	CreatedAt      time.Time
	TTLAt          time.Time `gorm:"column:ttl_at"`
}

func (IdempotencyKey) TableName() string { return "idempotency_keys" }

// IdempotencyHeaderName is the request header clients use to deduplicate
// retries. Stripe / GitHub / Square all use the same name; matching is
// case-insensitive at the HTTP layer.
const IdempotencyHeaderName = "Idempotency-Key"

const (
	maxIdempotencyKeyLen = 128
	maxBodySize          = 1 << 20 // 1 MiB — guards against an iPad uploading a giant payload
)

// Idempotency returns a middleware that caches the first response for a
// given (Idempotency-Key, endpoint) tuple. Subsequent requests with the
// same key replay the cached response; mismatched payloads under the
// same key return 409. The endpoint label scopes the dedup key so the
// same UUID can be reused across different operations.
//
// Behaviour:
//   - Missing header → falls through to the handler unchanged.
//   - Header present + cached row + body hash matches → replay the cached
//     status + body, abort the handler.
//   - Header present + cached row + body hash differs → 409 Conflict.
//   - Header present + no cached row → claim the row, run handler, store
//     the response, then return it.
//
// Production-grade enough for the bill close / alt-payment flows; if we
// outgrow the SQL backend we can swap in Redis without changing the call
// sites.
func Idempotency(db *gorm.DB, endpoint string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.GetHeader(IdempotencyHeaderName))
		if key == "" {
			c.Next()
			return
		}
		if len(key) > maxIdempotencyKeyLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Idempotency-Key too long"})
			c.Abort()
			return
		}

		// Buffer the body so the downstream handler can still read it.
		body, err := readAndRestoreBody(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
			c.Abort()
			return
		}

		hash := sha256Hex(body)

		// Look up an existing row. SELECT first to keep the common-case path
		// (cache hit) cheap; the INSERT happens later only on a miss.
		var existing IdempotencyKey
		err = db.Where("key = ? AND endpoint = ?", key, endpoint).
			Where("ttl_at > ?", time.Now()).
			First(&existing).Error

		if err == nil {
			if existing.RequestHash != hash {
				c.JSON(http.StatusConflict, gin.H{
					"error": "Idempotency-Key already used with a different request body",
				})
				c.Abort()
				return
			}
			// Replay the cached response. ResponseStatus==0 means the
			// original handler is still in flight — return 425 Too Early so
			// the client retries; we deliberately don't block here.
			if existing.ResponseStatus == 0 {
				c.JSON(http.StatusTooEarly, gin.H{
					"error": "Original request still in flight — retry shortly",
				})
				c.Abort()
				return
			}
			c.Data(existing.ResponseStatus, "application/json", existing.ResponseBody)
			c.Abort()
			return
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			// DB error — let the handler run unmemoized rather than blocking
			// a real operator action. We log via the gin error chain so the
			// observability layer picks it up.
			_ = c.Error(err)
			c.Next()
			return
		}

		// Claim the row with a pending (status=0) marker. ON CONFLICT means
		// two concurrent retries race to claim, and the loser will catch the
		// row on the SELECT replay path below.
		businessID := extractBusinessID(c)
		row := IdempotencyKey{
			Key:            key,
			Endpoint:       endpoint,
			BusinessID:     businessID,
			RequestHash:    hash,
			ResponseStatus: 0,
			TTLAt:          time.Now().Add(24 * time.Hour),
		}
		if err := db.Create(&row).Error; err != nil {
			// Conflict means another goroutine just claimed it. Fall back to
			// the replay path by recursing once.
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				c.JSON(http.StatusTooEarly, gin.H{
					"error": "Concurrent request with same Idempotency-Key — retry shortly",
				})
				c.Abort()
				return
			}
			_ = c.Error(err)
			c.Next()
			return
		}

		// Capture the handler's response so we can persist it for replay.
		writer := &capturingResponseWriter{ResponseWriter: c.Writer}
		c.Writer = writer

		c.Next()

		// Don't store partial responses if the handler aborted with no
		// status — let the row TTL out and the client retry.
		status := writer.Status()
		if status == 0 {
			status = http.StatusInternalServerError
		}

		// Auth-gate rejections (401/403) must not consume the key: the
		// credential lives in headers (X-Manager-Pin, bearer token), which
		// are not part of the request hash, so the same key + body can
		// legitimately succeed on retry once credentials are supplied.
		// Caching the rejection would replay it and permanently block the
		// retry (the manager-PIN probe→retry flow hits exactly this).
		// Delete the claimed row — leaving it pending (status=0) would send
		// the retry down the 425 still-in-flight path instead.
		// Only persist a terminal, deterministic, safe-to-replay outcome (2xx and
		// non-transient 4xx). Anything else releases the key so the operator's
		// automatic retry re-runs the handler instead of replaying a stale failure:
		//   - 401/403: credential lives in headers (not the request hash), so the
		//     same key+body can legitimately succeed once creds are supplied.
		//   - >=500: a transient infra error (DB hiccup) must not become a 24h hard
		//     fail on bill close/void/refund — the whole reason the key exists is to
		//     let the retry succeed once the condition clears.
		//   - 408/425/429: in-flight / too-early / rate-limited are retry-now signals,
		//     not final answers; caching them would replay the stall.
		if status == http.StatusUnauthorized ||
			status == http.StatusForbidden ||
			status == http.StatusRequestTimeout ||
			status == http.StatusTooEarly ||
			status == http.StatusTooManyRequests ||
			status >= http.StatusInternalServerError {
			if delErr := db.Delete(&IdempotencyKey{}, "id = ?", row.ID).Error; delErr != nil {
				_ = c.Error(delErr)
			}
			return
		}

		// Persist the captured response. Errors here aren't fatal — the
		// client already got their response — but we surface them via the
		// gin error chain so the observability layer can alert.
		if updateErr := db.Model(&IdempotencyKey{}).
			Where("id = ?", row.ID).
			Updates(map[string]interface{}{
				"response_status": status,
				"response_body":   writer.body.Bytes(),
			}).Error; updateErr != nil {
			_ = c.Error(updateErr)
		}
	}
}

// extractBusinessID pulls the business id from the gin context if a
// previous middleware stored it. Bill routes set "business_id" via
// RequireBillBusinessAccess; we don't require it (the dedup key is the
// authoritative scope) but storing it makes audit queries cheaper.
func extractBusinessID(c *gin.Context) *uint {
	if v, ok := c.Get("business_id"); ok {
		switch id := v.(type) {
		case uint:
			return &id
		case uint64:
			u := uint(id)
			return &u
		case int:
			if id >= 0 {
				u := uint(id)
				return &u
			}
		case int64:
			if id >= 0 {
				u := uint(id)
				return &u
			}
		}
	}
	return nil
}

// readAndRestoreBody reads c.Request.Body into memory and replaces it
// with a fresh ReadCloser so the downstream handler still sees the body.
func readAndRestoreBody(c *gin.Context) ([]byte, error) {
	if c.Request.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBodySize+1))
	if err != nil {
		return nil, err
	}
	_ = c.Request.Body.Close()
	if len(body) > maxBodySize {
		return nil, errors.New("request body too large for idempotency middleware")
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// capturingResponseWriter intercepts the response body so the middleware
// can persist it for replay. It still forwards everything to the actual
// gin writer so the client sees the original response without delay.
type capturingResponseWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *capturingResponseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *capturingResponseWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}
