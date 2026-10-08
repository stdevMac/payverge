package httpclientx

import (
	"context"
	"io"
)

// cancelOnClose ties a context.CancelFunc to the response body's Close so the
// per-feature deadline context is released exactly when the caller is done.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}
