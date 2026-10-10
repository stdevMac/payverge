package httpclientx

import (
	"errors"
	"net/url"
)

// RedactURLError strips everything but the scheme and host from the URL inside
// a *url.Error. http.Client.Do and Get return a *url.Error whose text embeds
// the full request URL, so a provider credential carried in the URL (an API
// key in the query, a bot token in the path) would otherwise reach logs and
// wrapped errors on every timeout, DNS or TLS failure. The method, host and
// underlying cause stay, and errors.Is/As on the cause (for example
// context.DeadlineExceeded) and Timeout() keep working. Wrappers around the
// *url.Error are dropped because their text repeats the URL. Any other error
// is returned unchanged.
func RedactURLError(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	return &url.Error{Op: ue.Op, URL: urlOrigin(ue.URL), Err: ue.Err}
}

// urlOrigin returns scheme://host for raw, or a fixed placeholder when raw
// does not parse (the unparsed text may itself hold the secret).
func urlOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "[redacted]"
	}
	return u.Scheme + "://" + u.Host
}
