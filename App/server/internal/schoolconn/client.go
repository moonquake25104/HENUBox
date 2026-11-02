package schoolconn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// Production endpoints. The host is fixed by the adapter: neither the UI nor
// the API accepts a school URL from callers. Tests point BaseURL at an
// httptest server instead.
const (
	ProductionBaseURL   = "https://xk.henu.edu.cn"
	pathShowYearTerm    = "/jw/common/showYearTerm.action"
	pathTimetableData   = "/wsxk/xkjg.ckdgxsxdkchj_data10319.jsp"
	refererTimetable    = ProductionBaseURL + "/student/xkjg.wdkb.jsp?menucode=S20301"
	maxResponseBytes    = 8 << 20
	requestTimeout      = 15 * time.Second
	maxSameHostRedirect = 3
	// userAgent identifies the server-side fetcher; it must stay truthful
	// about what it is rather than mimicking a full browser fingerprint.
	userAgent = "xique-assistant direct-fetch/1"
)

// Classification errors. Handlers map them to safe codes; none of them may
// carry a school URL in their message (net/http errors embed URLs, so the
// client wraps them into fixed-path labels).
var (
	ErrAuthRequired        = errors.New("school session requires re-login")
	ErrUpstreamUnavailable = errors.New("school temporarily unavailable")
	ErrUpstreamSafe        = errors.New("school returned an unexpected response")
	ErrResponseTooLarge    = errors.New("school response exceeds the size limit")
)

// Cookie is one school session cookie as delivered by the login helper.
type Cookie struct {
	Name     string     `json:"name"`
	Value    string     `json:"value"`
	Domain   string     `json:"domain"`
	Path     string     `json:"path"`
	Secure   bool       `json:"secure"`
	HTTPOnly bool       `json:"httpOnly"`
	Expires  *time.Time `json:"expires,omitempty"`
}

// Term is the verified current school term.
type Term struct {
	Year     string `json:"upstream_year"` // xn, e.g. 2026 meaning 2026-2027
	Term     string `json:"upstream_term"` // xqM, e.g. 0 meaning first semester
	Label    string `json:"label"`         // xqName, e.g. 第一学期
	UserCode string `json:"-"`             // internal school subject id, never serialized
}

// Client fetches read-only school data with an authorized session.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	Now     func() time.Time
}

// NewClient builds a client against baseURL with the fixed redirect policy,
// timeouts and limits. TLS verification is never disabled.
func NewClient(baseURL string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	host := base.Host
	transport := &http.Transport{
		// Same defaults as net/http plus an explicit proxy decision: use the
		// environment proxy like a normal Go program would.
		Proxy: http.ProxyFromEnvironment,
	}
	return &Client{
		BaseURL: baseURL,
		HTTP: &http.Client{
			Transport: transport,
			Jar:       jar,
			Timeout:   requestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxSameHostRedirect {
					return fmt.Errorf("stopped after %d redirects", maxSameHostRedirect)
				}
				if req.URL.Host != host {
					return fmt.Errorf("redirect crosses host boundary")
				}
				return nil
			},
		},
	}, nil
}

// SetSession installs the authorized cookies for the school origin.
func (c *Client) SetSession(cookies []Cookie) error {
	origin, err := url.Parse(c.BaseURL)
	if err != nil {
		return err
	}
	if c.HTTP.Jar == nil {
		return errors.New("client has no cookie jar")
	}
	// Only cookies whose domain matches the school host are installed; a
	// helper bug leaking foreign domains must not poison the jar.
	var filtered []*http.Cookie
	for _, ck := range cookies {
		d := strings.TrimPrefix(strings.ToLower(ck.Domain), ".")
		if d != "" && origin.Hostname() != d && !strings.HasSuffix(d, "."+origin.Hostname()) {
			continue
		}
		httpCookie := &http.Cookie{
			Name: ck.Name, Value: ck.Value, Domain: ck.Domain, Path: ck.Path,
			Secure: ck.Secure, HttpOnly: ck.HTTPOnly,
		}
		if ck.Expires != nil {
			httpCookie.Expires = *ck.Expires
		}
		filtered = append(filtered, httpCookie)
	}
	c.HTTP.Jar.SetCookies(origin, filtered)
	return nil
}

// loginPageMarkers are matched case-insensitively against response bodies to
// tell "the school bounced us to the CAS login" apart from other errors.
var loginPageMarkers = []string{
	"/cas/login.action",
	"/cas/logon.action",
	"name=\"randnumber\"",
	"请重新登录",
}

func looksLikeLoginPage(body []byte) bool {
	lower := strings.ToLower(string(body))
	for _, m := range loginPageMarkers {
		if strings.Contains(lower, strings.ToLower(m)) {
			return true
		}
	}
	return false
}

// get performs one capped GET; bodies larger than the limit abort.
func (c *Client) get(ctx context.Context, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, 0, ErrUpstreamSafe
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", refererTimetable)
	req.Header.Set("Accept", "text/html,application/json,text/plain,*/*")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		// The error text embeds the full URL; wrap without it.
		return nil, 0, fmt.Errorf("%w: network error on fixed path", ErrUpstreamUnavailable)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("%w: read error on fixed path", ErrUpstreamUnavailable)
	}
	if len(body) > maxResponseBytes {
		return nil, resp.StatusCode, ErrResponseTooLarge
	}
	if resp.StatusCode != http.StatusOK {
		if looksLikeLoginPage(body) {
			return nil, resp.StatusCode, ErrAuthRequired
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return nil, resp.StatusCode, fmt.Errorf("%w: school status %d", ErrUpstreamUnavailable, resp.StatusCode)
		}
		return nil, resp.StatusCode, fmt.Errorf("%w: school status %d", ErrUpstreamSafe, resp.StatusCode)
	}
	return body, resp.StatusCode, nil
}

// FetchTerm reads and validates the current school term. A session expired
// mid-flight usually shows up here as the CAS login page instead of JSON.
func (c *Client) FetchTerm(ctx context.Context) (*Term, error) {
	body, _, err := c.get(ctx, pathShowYearTerm)
	if err != nil {
		return nil, err
	}
	if looksLikeLoginPage(body) {
		return nil, ErrAuthRequired
	}
	var raw struct {
		XN       string `json:"xn"`
		XQM      string `json:"xqM"`
		XQName   string `json:"xqName"`
		UserCode string `json:"userCode"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: term response is not valid JSON", ErrUpstreamSafe)
	}
	if strings.TrimSpace(raw.XN) == "" || strings.TrimSpace(raw.XQM) == "" ||
		strings.TrimSpace(raw.XQName) == "" || strings.TrimSpace(raw.UserCode) == "" {
		return nil, fmt.Errorf("%w: term response missing required fields", ErrUpstreamSafe)
	}
	return &Term{
		Year: strings.TrimSpace(raw.XN), Term: strings.TrimSpace(raw.XQM),
		Label: strings.TrimSpace(raw.XQName), UserCode: strings.TrimSpace(raw.UserCode),
	}, nil
}

// BuildTimetableParams encodes the timetable query the way the school page's
// own JS does: Base64 of "xn=..&xq=..&xh=..", then URL query encoding of the
// whole Base64 string.
func BuildTimetableParams(term *Term) string {
	payload := fmt.Sprintf("xn=%s&xq=%s&xh=%s", term.Year, term.Term, term.UserCode)
	return url.QueryEscape(base64.StdEncoding.EncodeToString([]byte(payload)))
}

// FetchTimetable returns the raw GBK timetable bytes for the verified term.
// The bytes go to henu.Parse untouched: no transcoding, no charset fix-ups.
func (c *Client) FetchTimetable(ctx context.Context, term *Term) ([]byte, error) {
	body, _, err := c.get(ctx, pathTimetableData+"?params="+BuildTimetableParams(term))
	if err != nil {
		return nil, err
	}
	if looksLikeLoginPage(body) {
		return nil, ErrAuthRequired
	}
	return body, nil
}
