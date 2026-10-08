package handlers

import (
	"archive/zip"
	"bytes"
	"crypto/subtle"
	"net/http"
	"runtime/pprof"

	"github.com/gin-gonic/gin"
)

// RegisterPprofSnapshot wires GET /internal/_pprof_snapshot, which returns a
// zip containing goroutine + heap profiles. If token is empty the route is not
// registered (effectively disabled).
func RegisterPprofSnapshot(r gin.IRouter, token string) {
	if token == "" {
		return
	}
	r.GET("/internal/_pprof_snapshot", func(c *gin.Context) {
		provided := c.GetHeader("X-Pprof-Token")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		buf := &bytes.Buffer{}
		zw := zip.NewWriter(buf)

		for _, name := range []string{"goroutine", "heap"} {
			p := pprof.Lookup(name)
			if p == nil {
				continue
			}
			w, err := zw.Create(name + ".pprof")
			if err != nil {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			if err := p.WriteTo(w, 0); err != nil {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
		}
		if err := zw.Close(); err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Data(http.StatusOK, "application/zip", buf.Bytes())
	})
}
