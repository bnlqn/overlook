package fetch

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strings"
)

// Source is how Overlook reaches one source: its address, and its key or its
// login. An adapter fills one in its config's Check, readies it with its own
// Check, and reads it by path, so its key goes with every read and nowhere
// else (ADR 0013). It is safe to share between reads.
type Source struct {
	// URL is the source's address, each path read under it, its own path
	// kept, such as https://example.com/pihole.
	URL string
	// Header is sent with every read: a key in whatever header the source
	// takes, a user and password from Basic, or an Accept other than
	// application/json, such as text/calendar.
	Header http.Header
	// CA is a PEM file of the CA that signed the source's certificate, when
	// the system doesn't trust it, such as a Proxmox host's. There is no way
	// to skip the check, so a key is never sent to whoever answers in the
	// source's place.
	CA string
	// ServerName is the name the certificate is for, with CA, when URL
	// reaches the source by another, such as its address.
	ServerName string
	// Login is the source's one login, where it offers no other way in
	// (ADR 0009).
	Login *Login

	// client and session are Check's.
	client  *http.Client
	session *session
}

// Check refuses a source Overlook couldn't read, and readies it. Its error
// names the config's field, as "url is missing".
func (s *Source) Check() error {
	if err := CheckURL(s.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if s.ServerName != "" && s.CA == "" {
		return errors.New("server_name needs the ca that signed the certificate")
	}
	next := http.DefaultTransport
	if s.CA != "" {
		pem, err := os.ReadFile(s.CA) //nolint:gosec // the owner names their own CA
		if err != nil {
			return fmt.Errorf("ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return fmt.Errorf("ca: %s holds no PEM certificate", s.CA)
		}
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.TLSClientConfig = &tls.Config{RootCAs: pool, ServerName: s.ServerName, MinVersion: tls.VersionTLS12}
		next = t
	}
	var login string
	if s.Login != nil {
		u, err := url.Parse(strings.TrimSuffix(s.URL, "/") + s.Login.Path)
		if err != nil || !strings.HasPrefix(s.Login.Path, "/") {
			return errors.New("login's path needs to begin with /")
		}
		login = u.String()
	}
	s.client = &http.Client{Transport: guard{next: next, login: login}, CheckRedirect: sameOrigin}
	s.session = nil
	if s.Login != nil {
		s.session = &session{client: s.client, url: login, login: *s.Login}
	}
	return nil
}

// at is the address of path under the source's: path is empty, for the
// address itself, as it is, or begins with /, so that no read leaves the
// source, as "@elsewhere.example.com" would.
func (s Source) at(path string) (string, error) {
	switch {
	case path == "":
		return s.URL, nil
	case !strings.HasPrefix(path, "/"):
		return "", errors.New("a path that doesn't begin with /")
	}
	return strings.TrimSuffix(s.URL, "/") + path, nil
}

// JSON reads the answer at path into v. Its error is short enough for the
// page and never holds the address or the header, which may hold a key.
func (s Source) JSON(ctx context.Context, path string, v any) error {
	body, err := s.read(ctx, path, "application/json")
	if err != nil {
		return err
	}
	if err := Decode(body, v); err != nil {
		return fmt.Errorf("an answer that isn't the JSON expected: %w", err)
	}
	return nil
}

// Decode reads data into v, a pointer, as json.Unmarshal does, but leaves out
// each item of a list in data that v can't hold, such as one whose time is
// written in another format, and reads the rest: one odd item costs a row,
// never its source. Anything else v can't hold is an error. JSON reads every
// answer with it; an adapter that reads JSON inside an answer, as Tautulli's
// data, reads that with it too.
func Decode(data []byte, v any) error {
	err := json.Unmarshal(data, v)
	var invalid *json.InvalidUnmarshalError
	if err == nil || errors.As(err, &invalid) {
		return err
	}
	t := reflect.TypeOf(v).Elem()
	mended, ok := mend(data, func(b []byte) bool { return json.Unmarshal(b, reflect.New(t).Interface()) == nil })
	if !ok {
		return err
	}
	// What the first try read is dropped, since an item it read may be one
	// left out.
	reflect.ValueOf(v).Elem().SetZero()
	return json.Unmarshal(mended, v)
}

// mend gives back data less each item of its lists, however deep, that
// keeps it from fitting, and whether what's left fits. fits says whether a
// value fits in data's place, which lets an item be tried on its own, in its
// place, without knowing the type it's read into.
func mend(data []byte, fits func([]byte) bool) ([]byte, bool) {
	if fits(data) {
		return data, true
	}
	var mended any
	switch data = bytes.TrimSpace(data); {
	case bytes.HasPrefix(data, []byte("[")):
		var items []json.RawMessage
		if json.Unmarshal(data, &items) != nil {
			return nil, false
		}
		kept := []json.RawMessage{}
		for _, item := range items {
			if m, ok := mend(item, func(b []byte) bool { return fits(slices.Concat([]byte("["), b, []byte("]"))) }); ok {
				kept = append(kept, m)
			}
		}
		mended = kept
	case bytes.HasPrefix(data, []byte("{")):
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) != nil {
			return nil, false
		}
		for k, field := range fields {
			key, _ := json.Marshal(k)
			m, ok := mend(field, func(b []byte) bool { return fits(slices.Concat([]byte("{"), key, []byte(":"), b, []byte("}"))) })
			if !ok {
				return nil, false
			}
			fields[k] = m
		}
		mended = fields
	default:
		return nil, false
	}
	b, err := json.Marshal(mended)
	return b, err == nil && fits(b)
}

// Text reads the answer at path as it is, as JSON reads one, asking for
// whatever Header's Accept names, such as text/calendar.
func (s Source) Text(ctx context.Context, path string) ([]byte, error) {
	return s.read(ctx, path, "*/*")
}

// read reads the answer at path, asking for accept unless Header names
// another. With a login, it sends the session, and logs in first if there is
// none or the source refuses it.
func (s Source) read(ctx context.Context, path, accept string) ([]byte, error) {
	if s.client == nil {
		return nil, errors.New("a source read before its Check")
	}
	at, err := s.at(path)
	if err != nil {
		return nil, err
	}
	if s.session == nil {
		return s.get(ctx, at, accept, nil)
	}
	sent, err := s.session.header(ctx, nil)
	if err != nil {
		return nil, err
	}
	body, err := s.get(ctx, at, accept, sent)
	if s.session.refused(err) {
		if sent, err = s.session.header(ctx, sent); err != nil {
			return nil, err
		}
		body, err = s.get(ctx, at, accept, sent)
	}
	return body, err
}

// get sends a GET to at with Header and sent, the session's header, if any.
func (s Source) get(ctx context.Context, at, accept string, sent http.Header) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, at, nil)
	if err != nil {
		return nil, errors.New("bad url")
	}
	req.Header.Set("Accept", accept)
	for _, h := range []http.Header{s.Header, sent} {
		for k, vs := range h {
			req.Header[http.CanonicalHeaderKey(k)] = vs
		}
	}
	_, body, err := do(s.client, req)
	return body, err
}
