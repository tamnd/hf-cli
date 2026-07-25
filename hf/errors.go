package hf

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
)

// errors.go is the one place an HTTP status becomes a program outcome. Every
// surface, CLI exit code, HTTP response, and MCP error object, reads the kind
// set here, so a 404 means the same thing everywhere.

// statusError classifies a non-2xx response and pulls the hub's own message out
// of the body, which is usually more specific than anything we could write. The
// error field on a 400 from an expand parameter, for instance, enumerates the
// valid values.
func statusError(rawURL string, status int, body []byte) error {
	msg := hubMessage(body)
	where := shortURL(rawURL)
	switch {
	case status == http.StatusUnauthorized:
		return errs.NeedAuth("%s: authentication required%s", where, suffix(msg))
	case status == http.StatusForbidden:
		// A 403 on the hub is usually a gated repo you have not accepted, which
		// is an auth problem the user can act on, not a permanent failure.
		return errs.NeedAuth("%s: access denied%s", where, suffix(msg))
	case status == http.StatusNotFound || status == http.StatusGone:
		return errs.NotFound("%s: not found%s", where, suffix(msg))
	case status == http.StatusTooManyRequests:
		return errs.RateLimited("%s: rate limited%s", where, suffix(msg))
	case status == http.StatusNotImplemented:
		return errs.Unsupported("%s: not supported%s", where, suffix(msg))
	case status == http.StatusBadRequest:
		return errs.Usage("%s: bad request%s", where, suffix(msg))
	case status >= 500:
		return errs.New(errs.KindNetwork, "%s: server error %d%s", where, status, suffix(msg))
	default:
		return errs.New(errs.KindGeneric, "%s: http %d%s", where, status, suffix(msg))
	}
}

// hubMessage extracts the human-readable message from an error body. The hub
// uses several shapes for this, and the point of trying all of them is that the
// message is the most useful part of a failure.
func hubMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var env struct {
		Error  json.RawMessage `json:"error"`
		Detail json.RawMessage `json:"detail"`
		Cause  string          `json:"cause"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return firstLine(string(body))
	}
	for _, raw := range []json.RawMessage{env.Error, env.Detail} {
		if len(raw) == 0 {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			return s
		}
		var list []string
		if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
			return strings.Join(list, "; ")
		}
		return firstLine(string(raw))
	}
	return env.Cause
}

func suffix(msg string) string {
	if msg == "" {
		return ""
	}
	return ": " + msg
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}

// shortURL drops the scheme and host so an error message reads as a path rather
// than a wall of URL.
func shortURL(raw string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[i:]
	}
	if i := strings.IndexByte(s, '?'); i > 0 {
		s = s[:i]
	}
	return s
}

func wrapNetwork(rawURL string, err error) error {
	return errs.Wrap(errs.KindNetwork, err, "%s", shortURL(rawURL))
}

// IsNotFound reports whether an error came back as a 404. Several lookups are
// speculative by design, such as trying the organization route before the user
// route, so this is a normal control-flow question rather than an exception.
func IsNotFound(err error) bool { return errs.KindOf(err) == errs.KindNotFound }

// IsNeedAuth reports whether an error was a 401 or 403.
func IsNeedAuth(err error) bool { return errs.KindOf(err) == errs.KindNeedAuth }

func errorsAs(err error, target any) bool { return errors.As(err, target) }

func jitter(n int64) int64 {
	if n <= 0 {
		return 0
	}
	return rand.Int64N(n)
}
