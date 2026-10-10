package httpclientx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRedactURLError_dropsQueryAndPathSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 50 * time.Millisecond}
	_, err := client.Get(srv.URL + "/botSECRET-TOKEN/sendMessage?key=SECRET-KEY")
	require.Error(t, err)
	require.Contains(t, err.Error(), "SECRET-KEY", "precondition: the raw *url.Error carries the URL")

	redacted := RedactURLError(fmt.Errorf("call provider: %w", err))
	require.NotContains(t, redacted.Error(), "SECRET-KEY")
	require.NotContains(t, redacted.Error(), "SECRET-TOKEN")
	require.Contains(t, redacted.Error(), srv.URL, "scheme and host stay for diagnosis")

	var ue *url.Error
	require.True(t, errors.As(redacted, &ue))
	require.True(t, ue.Timeout(), "timeout classification survives redaction")
}

func TestRedactURLError_keepsCause(t *testing.T) {
	err := &url.Error{Op: "Post", URL: "https://api.example.com/v2?key=SECRET", Err: context.DeadlineExceeded}
	redacted := RedactURLError(err)
	require.Equal(t, `Post "https://api.example.com": context deadline exceeded`, redacted.Error())
	require.ErrorIs(t, redacted, context.DeadlineExceeded)
}

func TestRedactURLError_unparseableURL(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "%zz?key=SECRET", Err: errors.New("boom")}
	require.Equal(t, `Get "[redacted]": boom`, RedactURLError(err).Error())
}

func TestRedactURLError_passesOtherErrorsThrough(t *testing.T) {
	require.NoError(t, RedactURLError(nil))
	plain := errors.New("status 500")
	require.Same(t, plain, RedactURLError(plain))
}
