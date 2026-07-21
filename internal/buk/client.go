package buk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
)

// DefaultBaseURL is the Buk tenant for Robotia.
const DefaultBaseURL = "https://robotia.buk.cl"

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// ErrInvalidCredentials is returned when Buk rejects the login. Buk answers with
// a single generic message for both an unknown email and a wrong password, so
// the two cases are intentionally indistinguishable.
var ErrInvalidCredentials = errors.New("buk: invalid email or password")

// ErrLocked is returned when Buk's brute-force protection (Devise lockable) has
// locked the account after repeated failed attempts and requires an email unlock.
var ErrLocked = errors.New("buk: account locked, check email to unlock")

// invalidCredentialsMarker is the stable fragment of Buk's rejection alert
// ("Tu correo electrónico o contraseña no son correctos"). Both a wrong password
// and an unknown email re-render the sign-in page carrying it; a successful login
// renders the dashboard without it.
const invalidCredentialsMarker = "no son correctos"

// lockedMarker is the stable fragment of Buk's lockout alert ("Te enviamos un
// correo electrónico para desbloquear tu cuenta").
const lockedMarker = "desbloquear tu cuenta"

var (
	reToken = regexp.MustCompile(`name="authenticity_token"[^>]*value="([^"]+)"`)
	reMeta  = regexp.MustCompile(`<meta[^>]*name="csrf-token"[^>]*content="([^"]+)"`)
	reJobID = regexp.MustCompile(`id="current_job_id"[^>]*value="([^"]+)"`)
	reForm  = regexp.MustCompile(`(?s)id="web-marking-form".*?</form>`)
)

// Client drives the Buk employee-portal web-marking flow over plain HTTP,
// holding the authenticated session in its cookie jar.
type Client struct {
	http    *http.Client
	baseURL string
}

// Portal holds the state scraped from the authenticated portal that is needed
// to submit a marcaje.
type Portal struct {
	JobID     string
	CSRFToken string
	FormToken string
}

// MarkResult reports the outcome of a marcaje POST as understood from Buk's response.
type MarkResult struct {
	Accepted  bool
	Duplicate bool
	Message   string
}

// New builds a Client against the default Robotia tenant with a fresh cookie jar.
func New() (*Client, error) {
	return NewWithBaseURL(DefaultBaseURL)
}

// NewWithBaseURL builds a Client against an explicit base URL, useful for tests.
func NewWithBaseURL(baseURL string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("buk: cookie jar: %w", err)
	}
	client := &http.Client{Jar: jar}
	client.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 15 {
			return http.ErrUseLastResponse
		}
		return nil
	}
	return &Client{http: client, baseURL: strings.TrimRight(baseURL, "/")}, nil
}

// Login performs the two-step Devise login. Buk always advances to the password
// form regardless of whether the email exists, and re-renders the sign-in page
// on any failure; success is therefore detected by leaving the auth pages.
func (c *Client) Login(ctx context.Context, email, password string) error {
	signInBody, _, err := c.request(ctx, http.MethodGet, "/users/sign_in", "", nil)
	if err != nil {
		return fmt.Errorf("buk: load sign-in: %w", err)
	}
	emailToken := tokenAfter(signInBody, `id="login-form"`)
	if emailToken == "" {
		emailToken = firstToken(signInBody)
	}
	if emailToken == "" {
		return errors.New("buk: could not read email-step authenticity token")
	}

	emailStep := url.Values{
		"utf8": {"✓"}, "authenticity_token": {emailToken}, "fragment": {""},
		"user[email]": {email}, "commit": {"Next"},
	}
	pwdBody, _, err := c.postForm(ctx, "/users/login", emailStep, nil)
	if err != nil {
		return fmt.Errorf("buk: email step: %w", err)
	}
	pwdToken := tokenAfter(pwdBody, `id="new_user"`)
	if pwdToken == "" {
		pwdToken = firstToken(pwdBody)
	}
	if pwdToken == "" {
		return errors.New("buk: unexpected email-step response")
	}

	pwdStep := url.Values{
		"utf8": {"✓"}, "authenticity_token": {pwdToken}, "login_email": {email},
		"user[email]": {email}, "user[password]": {password}, "commit": {"Sign In"},
	}
	afterLogin, _, err := c.postForm(ctx, "/users/sign_in", pwdStep, nil)
	if err != nil {
		return fmt.Errorf("buk: password step: %w", err)
	}
	lowered := strings.ToLower(afterLogin)
	if strings.Contains(lowered, lockedMarker) {
		return ErrLocked
	}
	if strings.Contains(lowered, invalidCredentialsMarker) {
		return ErrInvalidCredentials
	}
	return nil
}

// LoadPortal fetches the authenticated portal and scrapes the job id and CSRF
// tokens required to submit a marcaje.
func (c *Client) LoadPortal(ctx context.Context) (Portal, error) {
	body, _, err := c.request(ctx, http.MethodGet, "/static_pages/portal", "", nil)
	if err != nil {
		return Portal{}, fmt.Errorf("buk: load portal: %w", err)
	}
	form := reForm.FindString(body)
	portal := Portal{
		JobID:     firstGroup(reJobID, body),
		CSRFToken: firstGroup(reMeta, body),
		FormToken: firstToken(form),
	}
	if portal.JobID == "" || form == "" {
		return Portal{}, errors.New("buk: web-marking widget not present; session not authenticated")
	}
	if portal.CSRFToken == "" {
		portal.CSRFToken = portal.FormToken
	}
	return portal, nil
}

// Mark submits a marcaje for the given sentido ("ENTRADA" or "SALIDA"). Buk answers
// 204 both for a successful mark and for a business rejection (e.g. two marks in the
// same direction); the outcome is read from the x-ic-script response header.
func (c *Client) Mark(ctx context.Context, p Portal, sentido string) (MarkResult, error) {
	form := url.Values{
		"utf8": {"✓"}, "authenticity_token": {p.FormToken},
		"latitude": {""}, "longitude": {""},
		"job_id": {p.JobID}, "default_job_id": {p.JobID}, "_method": {"POST"},
	}
	headers := map[string]string{
		"X-CSRF-Token":           p.CSRFToken,
		"X-Requested-With":       "XMLHttpRequest",
		"X-IC-Request":           "true",
		"X-HTTP-Method-Override": "POST",
		"Accept":                 "text/html-partial, */*; q=0.9",
	}
	path := "/employee_portal/web_marking/marcaje?sentido=" + url.QueryEscape(sentido)

	req, err := c.newRequest(ctx, http.MethodPost, path, strings.NewReader(form.Encode()), headers)
	if err != nil {
		return MarkResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return MarkResult{}, fmt.Errorf("buk: mark %s: %w", sentido, err)
	}
	defer drain(resp.Body)

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return MarkResult{}, fmt.Errorf("buk: mark %s: unexpected status %d", sentido, resp.StatusCode)
	}
	return parseMarkResult(resp.Header.Get("x-ic-script")), nil
}

func (c *Client) request(ctx context.Context, method, path, body string, headers map[string]string) (string, string, error) {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := c.newRequest(ctx, method, path, reader, headers)
	if err != nil {
		return "", "", err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer drain(resp.Body)
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	return string(raw), resp.Request.URL.String(), nil
}

func (c *Client) postForm(ctx context.Context, path string, form url.Values, headers map[string]string) (string, string, error) {
	return c.request(ctx, http.MethodPost, path, form.Encode(), headers)
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("buk: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", c.baseURL)
	req.Header.Set("Referer", c.baseURL+"/static_pages/portal")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

func parseMarkResult(icScript string) MarkResult {
	lowered := strings.ToLower(icScript)
	switch {
	case strings.Contains(lowered, "exitoso") || strings.Contains(lowered, "success"):
		return MarkResult{Accepted: true, Message: "marcaje exitoso"}
	case strings.Contains(lowered, "mismo sentido") || strings.Contains(lowered, "dos marcajes"):
		return MarkResult{Accepted: true, Duplicate: true, Message: "marcaje duplicado en el mismo sentido"}
	default:
		return MarkResult{Accepted: true, Message: strings.TrimSpace(icScript)}
	}
}

func tokenAfter(html, marker string) string {
	idx := strings.Index(html, marker)
	if idx < 0 {
		return ""
	}
	return firstGroup(reToken, html[idx:])
}

func firstToken(html string) string {
	return firstGroup(reToken, html)
}

func firstGroup(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func drain(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}
