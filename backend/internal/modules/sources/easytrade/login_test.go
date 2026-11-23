package easytrade

// Login error handling (#830 follow-up): a fake Easytrade answering like the
// real API (checked with a dummy account): an unknown user is 401 with a
// message, a malformed field is 422 with an "errors" object. No network
// beyond httptest, no database.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

// loginFake answers /auth/login with a fixed status and body and records
// what was sent.
type loginFake struct {
	mu     sync.Mutex
	status int
	body   string
	sent   map[string]any
}

func (f *loginFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/auth/login" {
		f.sent = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&f.sent)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.status)
	_, _ = w.Write([]byte(f.body))
}

func loginServer(t *testing.T, status int, body string) (*loginFake, *httptest.Server) {
	t.Helper()
	f := &loginFake{status: status, body: body}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

func TestLogin401BadCredentialsWithMessage(t *testing.T) {
	_, srv := loginServer(t, http.StatusUnauthorized, `{"message":"E-posta veya şifre hatalı."}`)
	_, err := SignIn(context.Background(), srv.URL, srv.Client(), "a@b.c", "secret")
	if !errors.Is(err, ErrBadLogin) || errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrBadLogin", err)
	}
	var le *LoginError
	if !errors.As(err, &le) {
		t.Fatalf("not a LoginError: %v", err)
	}
	host := strings.TrimPrefix(srv.URL, "http://")
	if le.Host != host || le.Status != http.StatusUnauthorized {
		t.Fatalf("host/status = %q %d, want %q 401", le.Host, le.Status, host)
	}
	want := "Easytrade (" + host + ") 401: E-posta veya şifre hatalı."
	if got := le.UserMessage(); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestLogin422InvalidInputWithField(t *testing.T) {
	_, srv := loginServer(t, http.StatusUnprocessableEntity,
		`{"message":"Gönderdiğiniz veriler geçersiz.","errors":{"password":["Şifre en az 4 karakter olmalıdır."],"email":["E-posta geçersiz."]}}`)
	_, err := SignIn(context.Background(), srv.URL, srv.Client(), "a@b.c", "x")
	if !errors.Is(err, ErrInvalidInput) || errors.Is(err, ErrBadLogin) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	var le *LoginError
	if !errors.As(err, &le) {
		t.Fatalf("not a LoginError: %v", err)
	}
	if le.Field != "password" || le.Reason() != "Şifre: Şifre en az 4 karakter olmalıdır." {
		t.Fatalf("field = %q reason = %q", le.Field, le.Reason())
	}
	if !strings.HasSuffix(le.UserMessage(), " 422: Şifre: Şifre en az 4 karakter olmalıdır.") {
		t.Fatalf("message = %q", le.UserMessage())
	}
}

// The error carries the host only: no path, query or user info of the
// configured base URL.
func TestLoginErrorHostOnly(t *testing.T) {
	_, srv := loginServer(t, http.StatusUnauthorized, `{"message":"E-posta veya şifre hatalı."}`)
	u, _ := url.Parse(srv.URL)
	base := "http://user:pass@" + u.Host + "/?k=v"
	_, err := SignIn(context.Background(), base, srv.Client(), "a@b.c", "secret")
	var le *LoginError
	if !errors.As(err, &le) {
		t.Fatalf("not a LoginError: %v", err)
	}
	if le.Host != u.Host || le.Status != http.StatusUnauthorized {
		t.Fatalf("host/status = %q %d", le.Host, le.Status)
	}
	if strings.Contains(le.UserMessage(), "pass") || strings.Contains(le.UserMessage(), "k=v") {
		t.Fatalf("leaks: %q", le.UserMessage())
	}
}

func TestLoginNormalizesEmailKeepsPassword(t *testing.T) {
	f, srv := loginServer(t, http.StatusUnauthorized, `{"message":"E-posta veya şifre hatalı."}`)
	_, _ = SignIn(context.Background(), srv.URL, srv.Client(), "  Ofis@Ornek.COM ", " Gizli Şifre ")
	if f.sent["email"] != "ofis@ornek.com" {
		t.Fatalf("email sent = %q", f.sent["email"])
	}
	if f.sent["password"] != " Gizli Şifre " {
		t.Fatalf("password sent = %q", f.sent["password"])
	}
	if f.sent["device_name"] == nil || f.sent["revoke_other_tokens"] != false {
		t.Fatalf("body = %v", f.sent)
	}
}

func TestFirstFieldErrorKeepsOrder(t *testing.T) {
	for raw, want := range map[string][2]string{
		`{"email":["E-posta geçersiz."],"password":["kısa"]}`: {"email", "E-posta geçersiz."},
		`{"device_type":"zorunlu"}`:                           {"device_type", "zorunlu"},
		`{"password":[]}`:                                     {"", ""},
		``:                                                    {"", ""},
	} {
		f, m := firstFieldError(json.RawMessage(raw))
		if f != want[0] || m != want[1] {
			t.Errorf("%s → %q %q, want %q", raw, f, m, want)
		}
	}
	if FieldLabel("email") != "E-posta" || FieldLabel("password") != "Şifre" || FieldLabel("x") != "x" {
		t.Error("field labels")
	}
}

// An empty ID is the company Easytrade names at sign-in; an entered ID
// that is not the user's company is the mismatch error; the e-mail is
// trimmed and lowercased on the way.
func TestProbeOptionalCompanyID(t *testing.T) {
	ctx := context.Background()
	f := testfakes.EasytradeSample()
	srv := testfakes.Serve(t, f)
	svc := NewService(nil, nil, srv.URL, srv.Client())
	res, err := svc.Test(ctx, " A@B.C ", testfakes.EasytradePassword, "  ")
	if err != nil || res.CompanyID != testfakes.EasytradeCompanyID || res.CompanyName == "" || len(res.Branches) != 2 {
		t.Fatalf("empty ID: %+v %v", res, err)
	}
	if _, err := svc.Test(ctx, "a@b.c", testfakes.EasytradePassword, "43"); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("other ID: %v", err)
	}
	if _, err := svc.Test(ctx, "a@b.c", "x", ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("short password: %v", err)
	}
	if _, err := svc.Test(ctx, "a@b.c", "nope", ""); !errors.Is(err, ErrBadLogin) {
		t.Fatalf("wrong password: %v", err)
	}
}

// Without an ID and without a company in Easytrade's answer the user is
// asked for the ID.
func TestProbeNoCompanyNamed(t *testing.T) {
	_, srv := loginServer(t, http.StatusOK, `{"success":true,"data":{"token":"tok"}}`)
	svc := NewService(nil, nil, srv.URL, srv.Client())
	if _, err := svc.Test(context.Background(), "a@b.c", "pw", ""); !errors.Is(err, ErrNoCompany) {
		t.Fatalf("err = %v, want ErrNoCompany", err)
	}
}

func TestLogin403(t *testing.T) {
	for name, c := range map[string]struct {
		body, msg string
		want      error
		not       error
	}{
		"license expired": {`{"message":"Lisans süresi doldu","error_code":"LICENSE_EXPIRED"}`, "403: Easytrade lisansınızın süresi dolmuş.", ErrLicense, ErrForbidden},
		"no permission":   {`{"message":"Bu kanal için yetkiniz yok."}`, "403: Bu kanal için yetkiniz yok.", ErrForbidden, ErrLicense},
	} {
		_, srv := loginServer(t, http.StatusForbidden, c.body)
		_, err := SignIn(context.Background(), srv.URL, srv.Client(), "a@b.c", "secret")
		if !errors.Is(err, c.want) || errors.Is(err, c.not) || errors.Is(err, ErrBadLogin) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
			continue
		}
		var le *LoginError
		if !errors.As(err, &le) || !strings.HasSuffix(le.UserMessage(), c.msg) || le.Status != http.StatusForbidden {
			t.Errorf("%s: %+v", name, le)
		}
	}
}

// The API answer carries the code, Easytrade's text, the host and the
// status apart.
func TestLoginErrResponse(t *testing.T) {
	for _, c := range []struct {
		status     int
		body, code string
		http       int
	}{
		{401, `{"message":"E-posta veya şifre hatalı."}`, "EASYTRADE_BAD_CREDENTIALS", 422},
		{422, `{"message":"Gönderdiğiniz veriler geçersiz.","errors":{"password":["Şifre en az 4 karakter olmalıdır."]}}`, "EASYTRADE_INVALID_INPUT", 422},
		{403, `{"error_code":"LICENSE_EXPIRED"}`, "EASYTRADE_LICENSE_EXPIRED", 502},
		{403, `{"message":"Bu kanal için yetkiniz yok."}`, "EASYTRADE_FORBIDDEN", 422},
	} {
		_, srv := loginServer(t, c.status, c.body)
		host := strings.TrimPrefix(srv.URL, "http://")
		_, err := SignIn(context.Background(), srv.URL, srv.Client(), "a@b.c", "secret")
		rec := httptest.NewRecorder()
		writeErr(rec, httptest.NewRequest(http.MethodPost, "/", nil), err)
		var out struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Details []struct {
					Field, Message, Code string
				} `json:"details"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != c.http || out.Error.Code != c.code {
			t.Errorf("%d: %d %s", c.status, rec.Code, out.Error.Code)
			continue
		}
		if !strings.HasPrefix(out.Error.Message, "Easytrade ("+host+") "+strconv.Itoa(c.status)+": ") {
			t.Errorf("%d: message %q", c.status, out.Error.Message)
		}
		got := map[string]string{}
		for _, d := range out.Error.Details {
			got[d.Code] = d.Message
		}
		if got["EASYTRADE_HOST"] != host || got["EASYTRADE_STATUS"] != strconv.Itoa(c.status) {
			t.Errorf("%d: details %v", c.status, got)
		}
		if c.status == 422 && got["EASYTRADE_FIELD"] != "Şifre: Şifre en az 4 karakter olmalıdır." {
			t.Errorf("422: field detail %v", got)
		}
	}
}

// Easytrade's documented login answer has user (and its company) at the
// top level, not under data; an empty ID is that company.
func TestSignInTopLevelUser(t *testing.T) {
	_, srv := loginServer(t, http.StatusOK,
		`{"success":true,"data":{"token":"tok","expires_at":"2030-01-01T00:00:00Z"},"user":{"company":{"id":77,"name":"Deniz Ticaret"}}}`)
	s, err := SignIn(context.Background(), srv.URL, srv.Client(), "a@b.c", "pw")
	if err != nil || s.CompanyID != "77" || s.CompanyName != "Deniz Ticaret" {
		t.Fatalf("session %+v %v", s, err)
	}
}

// Review bug 1: only 401, 403 and a 422 on email or password are a refused
// sign-in; a 400 or a 422 on another field is an HTTPError with
// Easytrade's text, the host and the status.
func TestLoginOtherStatusesAreHTTPErrors(t *testing.T) {
	for _, c := range []struct {
		status int
		body   string
	}{
		{400, `{"success":false,"message":"İstek hatalı.","data":null}`},
		{422, `{"message":"Gönderdiğiniz veriler geçersiz.","errors":{"device_type":["Cihaz türü android ya da ios olmalıdır."]}}`},
		{500, `<html>Server Error</html>`},
	} {
		_, srv := loginServer(t, c.status, c.body)
		_, err := SignIn(context.Background(), srv.URL, srv.Client(), "a@b.c", "secret")
		var he *HTTPError
		if !errors.As(err, &he) || errors.Is(err, ErrBadLogin) || errors.Is(err, ErrInvalidInput) {
			t.Errorf("%d: err = %v, want an HTTPError", c.status, err)
			continue
		}
		if he.Status != c.status || he.Host != strings.TrimPrefix(srv.URL, "http://") {
			t.Errorf("%d: %+v", c.status, he)
		}
		rec := httptest.NewRecorder()
		writeErr(rec, httptest.NewRequest(http.MethodPost, "/", nil), err)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "EASYTRADE_LOGIN_REJECTED") ||
			!strings.Contains(rec.Body.String(), ") "+strconv.Itoa(c.status)+": ") {
			t.Errorf("%d: response %d %s", c.status, rec.Code, rec.Body.String())
		}
	}
	// A 422 on email or password is still the invalid-input refusal, even
	// when another field comes first.
	_, srv := loginServer(t, 422, `{"errors":{"device_type":["x"],"email":["E-posta geçerli bir adres olmalıdır."]}}`)
	_, err := SignIn(context.Background(), srv.URL, srv.Client(), "a@b.c", "secret")
	var le *LoginError
	if !errors.Is(err, ErrInvalidInput) || !errors.As(err, &le) || le.Reason() != "E-posta: E-posta geçerli bir adres olmalıdır." {
		t.Fatalf("422 email: %v", err)
	}
}

// Review bug 2: the token alone decides; success:false next to a token
// (Easytrade's documented 200 example) is a sign-in. No token, or a body
// we cannot read, is EASYTRADE_BAD_RESPONSE, never a wrong password.
func TestSignInTokenDecides(t *testing.T) {
	_, srv := loginServer(t, http.StatusOK, `{"success":false,"data":{"token":"tok","token_type":"Bearer","expires_in":3600},"user":{"company":{"id":"9"}}}`)
	s, err := SignIn(context.Background(), srv.URL, srv.Client(), "a@b.c", "pw")
	if err != nil || s.Token.Value != "tok" || s.CompanyID != "9" {
		t.Fatalf("success:false with a token: %+v %v", s, err)
	}
	for _, body := range []string{`{"success":true,"data":{}}`, `<html>bakımdayız</html>`, `{"data":{"token":5}}`} {
		_, srv := loginServer(t, http.StatusOK, body)
		_, err := SignIn(context.Background(), srv.URL, srv.Client(), "a@b.c", "pw")
		if !errors.Is(err, ErrBadResponse) || errors.Is(err, ErrBadLogin) {
			t.Errorf("%s: %v", body, err)
			continue
		}
		rec := httptest.NewRecorder()
		writeErr(rec, httptest.NewRequest(http.MethodPost, "/", nil), err)
		if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "EASYTRADE_BAD_RESPONSE") ||
			!strings.Contains(rec.Body.String(), "EASYTRADE_HOST") {
			t.Errorf("%s: response %d %s", body, rec.Code, rec.Body.String())
		}
	}
}

// Review bug 3: the documented user has company.id and is_active as
// strings and roles as an object; none of it breaks the decode.
func TestSignInDocumentedUserShape(t *testing.T) {
	_, srv := loginServer(t, http.StatusOK, `{"success":true,"data":{"token":"tok","token_type":"Bearer","expires_in":"2592000"},
		"user":{"id":"5","name":"Ali","roles":{"admin":true,"list":["x"]},"company":{"id":"123","name":"Deniz Ticaret","is_active":"1","extra":{"a":[1]}}}}`)
	s, err := SignIn(context.Background(), srv.URL, srv.Client(), "a@b.c", "pw")
	if err != nil || s.CompanyID != "123" || s.CompanyName != "Deniz Ticaret" {
		t.Fatalf("session %+v %v", s, err)
	}
	for raw, want := range map[string]string{`"123"`: "123", `123`: "123", `null`: "", `{"x":1}`: ""} {
		var id flexID
		if err := json.Unmarshal([]byte(raw), &id); err != nil || string(id) != want {
			t.Errorf("%s → %q %v", raw, id, err)
		}
	}
}

// A refresh answer with user null keeps the company named at login.
func TestSignInRefreshNullUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/refresh" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"token":"tok2","token_type":"Bearer","expires_in":3600},"user":null}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"token":"tok"},"user":{"company":{"id":"31","name":"Ege Gıda"}}}`))
	}))
	t.Cleanup(srv.Close)
	s, err := SignIn(context.Background(), srv.URL, srv.Client(), "a@b.c", "pw")
	if err != nil || s.Token.Value != "tok2" || s.CompanyID != "31" || s.CompanyName != "Ege Gıda" {
		t.Fatalf("session %+v %v", s, err)
	}
}

// No answer at all still names the server that was called.
func TestNetErrorCarriesHost(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	_, err := SignIn(context.Background(), dead.URL, nil, "a@b.c", "pw")
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v", err)
	}
	rec := httptest.NewRecorder()
	writeErr(rec, httptest.NewRequest(http.MethodPost, "/", nil), err)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), strings.TrimPrefix(dead.URL, "http://")) {
		t.Fatalf("response %d %s", rec.Code, rec.Body.String())
	}
}
