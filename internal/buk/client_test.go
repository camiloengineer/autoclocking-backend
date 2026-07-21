package buk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	signInHTML = `<form class="simple_form new_user" id="login-form" action="/users/login" method="post">` +
		`<input type="hidden" name="authenticity_token" value="EMAIL_STEP_TOKEN" autocomplete="off">` +
		`<input type="email" name="user[email]" id="user_email"></form>`

	passwordHTML = `<form class="simple_form new_user" id="new_user" action="/users/sign_in" method="post">` +
		`<input type="hidden" name="authenticity_token" value="PASSWORD_STEP_TOKEN" autocomplete="off">` +
		`<input type="hidden" name="login_email" value="cgonzalez@robotia.cl">` +
		`<input type="password" name="user[password]" id="user_password"></form>`

	failureHTML = signInHTML +
		`<div class="alert alert_warning">Tu correo electrónico o contraseña no son correctos.</div>`

	lockedHTML = signInHTML +
		`<div class="alert alert_warning">Múltiples intentos de inicio de sesión fallidos. Te enviamos un correo electrónico para desbloquear tu cuenta.</div>`

	portalHTML = `<meta name="csrf-token" content="PORTAL_META_CSRF">` +
		`<div id="web-marking-widget"><form id="web-marking-form" action="#" method="post">` +
		`<input type="hidden" name="authenticity_token" value="PORTAL_FORM_TOKEN" autocomplete="off">` +
		`<input type="hidden" name="latitude" id="latitude" value="">` +
		`<input type="hidden" name="job_id" id="current_job_id" value="2690" autocomplete="off">` +
		`<button ic-post-to="/employee_portal/web_marking/marcaje?sentido=ENTRADA">Entrada</button></form></div>`
)

// newTestServer mimics Buk: the email step always yields the password form. The
// password step outcome depends on `outcome`: "ok" redirects out of the auth
// pages to the portal; "invalid" and "locked" re-render /users/sign_in with the
// matching warning alert.
func newTestServer(t *testing.T, outcome string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/users/sign_in", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(signInHTML))
			return
		}
		switch outcome {
		case "ok":
			http.SetCookie(w, &http.Cookie{Name: "_Buk_session", Value: "authenticated"})
			http.Redirect(w, r, "/static_pages/portal", http.StatusFound)
		case "locked":
			_, _ = w.Write([]byte(lockedHTML))
		default:
			_, _ = w.Write([]byte(failureHTML))
		}
	})
	mux.HandleFunc("/users/login", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(passwordHTML))
	})
	mux.HandleFunc("/static_pages/portal", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(portalHTML))
	})
	mux.HandleFunc("/employee_portal/web_marking/marcaje", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-ic-script", "Buk.notyalert.notyAlertComplete('exitoso','Tu marcaje de asistencia ha sido exitoso.')")
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	client, err := NewWithBaseURL(baseURL)
	if err != nil {
		t.Fatalf("NewWithBaseURL: %v", err)
	}
	return client
}

func TestLoginPortalMarkFlow(t *testing.T) {
	server := newTestServer(t, "ok")
	client := newTestClient(t, server.URL)
	ctx := context.Background()

	if err := client.Login(ctx, "cgonzalez@robotia.cl", "robotia.."); err != nil {
		t.Fatalf("Login: %v", err)
	}

	portal, err := client.LoadPortal(ctx)
	if err != nil {
		t.Fatalf("LoadPortal: %v", err)
	}
	if portal.JobID != "2690" {
		t.Errorf("JobID = %q, want 2690", portal.JobID)
	}
	if portal.CSRFToken != "PORTAL_META_CSRF" {
		t.Errorf("CSRFToken = %q, want PORTAL_META_CSRF", portal.CSRFToken)
	}
	if portal.FormToken != "PORTAL_FORM_TOKEN" {
		t.Errorf("FormToken = %q, want PORTAL_FORM_TOKEN", portal.FormToken)
	}

	result, err := client.Mark(ctx, portal, "ENTRADA")
	if err != nil {
		t.Fatalf("Mark: %v", err)
	}
	if !result.Accepted || result.Duplicate {
		t.Errorf("Mark result = %+v, want accepted non-duplicate", result)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	server := newTestServer(t, "invalid")
	client := newTestClient(t, server.URL)

	err := client.Login(context.Background(), "cgonzalez@robotia.cl", "wrong")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login error = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginLocked(t *testing.T) {
	server := newTestServer(t, "locked")
	client := newTestClient(t, server.URL)

	err := client.Login(context.Background(), "cgonzalez@robotia.cl", "robotia..")
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("Login error = %v, want ErrLocked", err)
	}
}

func TestParseMarkResult(t *testing.T) {
	cases := []struct {
		name         string
		script       string
		wantAccepted bool
		wantDup      bool
	}{
		{"success", "marcaje exitoso", true, false},
		{"duplicate", "Registraste dos marcajes seguidos en el mismo sentido.", true, true},
		{"unknown", "algo raro", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseMarkResult(tc.script)
			if got.Accepted != tc.wantAccepted || got.Duplicate != tc.wantDup {
				t.Errorf("parseMarkResult(%q) = %+v", tc.script, got)
			}
		})
	}
}
