package soulstream

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The administration lane, measured against a stand-in for the surface it
// speaks to: the person's own bearer on every call, the surface's refusals
// kept in the surface's own words, and no surface at all when the
// deployment declares none.

// stubSurface is an administration surface that records what it was asked
// and answers what the test told it to.
type stubSurface struct {
	bearer string // the Authorization header of the last call
	method string
	path   string
	body   string

	status int
	answer string
}

func (s *stubSurface) serve(w http.ResponseWriter, r *http.Request) {
	s.bearer = r.Header.Get("Authorization")
	s.method, s.path = r.Method, r.URL.Path
	raw := make([]byte, r.ContentLength)
	if r.ContentLength > 0 {
		_, _ = r.Body.Read(raw)
	}
	s.body = string(raw)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s.status)
	_, _ = w.Write([]byte(s.answer))
}

// sessionOn builds a session whose reach points at the stand-in, the way a
// signed-in person's would.
func sessionOn(t *testing.T, s *stubSurface, bearer string) (*Session, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	sp := &Support{cfg: Config{AdminBase: srv.URL}, adminCl: srv.Client()}
	sp.adminCl.Timeout = 5 * time.Second
	return &Session{Persona: "owner", sp: sp, bearer: bearer}, srv
}

// The list: the person's own bearer carries it, and the surface's shapes
// arrive whole.
func TestTheListRidesThePersonsOwnBearer(t *testing.T) {
	s := &stubSurface{status: http.StatusOK, answer: `[
		{"id":"u-1","username":"owner","display_name":"Daan","status":"active",
		 "groups":["admin","realm"],"credentials":1},
		{"id":"u-2","username":"avery","status":"disabled","credentials":0}]`}
	sess, _ := sessionOn(t, s, "tok-abc")

	people, err := sess.Admin().People(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.bearer != "Bearer tok-abc" {
		t.Errorf("the call carried %q, not the person's own bearer", s.bearer)
	}
	if s.method != http.MethodGet || s.path != "/api/admin/users" {
		t.Errorf("the call went %s %s", s.method, s.path)
	}
	if len(people) != 2 {
		t.Fatalf("read %d people, want 2", len(people))
	}
	if !people[0].Active() || people[0].DisplayName != "Daan" ||
		len(people[0].Groups) != 2 || people[0].Credentials != 1 {
		t.Errorf("the first person came back as %+v", people[0])
	}
	if people[1].Active() {
		t.Errorf("a disabled person reads as able to sign in: %+v", people[1])
	}
}

// The invite: minted for a named person, and the token is handed back whole
// — it exists nowhere else.
func TestTheInviteComesBackWhole(t *testing.T) {
	s := &stubSurface{status: http.StatusCreated,
		answer: `{"invite":"sfi_deadbeef","enroll_url":"http://as/enroll?invite=sfi_deadbeef"}`}
	sess, _ := sessionOn(t, s, "tok-abc")

	inv, err := sess.Admin().MintInvite(context.Background(), "avery")
	if err != nil {
		t.Fatal(err)
	}
	if s.path != "/api/admin/invites" || s.method != http.MethodPost {
		t.Errorf("the call went %s %s", s.method, s.path)
	}
	var asked map[string]any
	if err := json.Unmarshal([]byte(s.body), &asked); err != nil {
		t.Fatalf("the call body is not the surface's shape: %q", s.body)
	}
	if asked["username"] != "avery" {
		t.Errorf("the invite was asked for %v", asked["username"])
	}
	if inv.Token != "sfi_deadbeef" || !strings.Contains(inv.URL, inv.Token) {
		t.Errorf("the invite came back as %+v", inv)
	}
}

// Taking a sign-in away names the person in the path and says what it wants
// in the surface's own words.
func TestTakingASignInAwayNamesThePerson(t *testing.T) {
	s := &stubSurface{status: http.StatusOK, answer: `{"status":"disabled"}`}
	sess, _ := sessionOn(t, s, "tok-abc")

	if err := sess.Admin().Disable(context.Background(), "avery"); err != nil {
		t.Fatal(err)
	}
	if s.path != "/api/admin/users/avery/status" {
		t.Errorf("the call went to %s", s.path)
	}
	if !strings.Contains(s.body, `"disabled"`) {
		t.Errorf("the call asked for %q", s.body)
	}
}

// A refusal is the surface's, kept whole: the words it used, and the
// difference between "not yours to do" and "that did not work".
func TestARefusalKeepsTheSurfacesOwnWords(t *testing.T) {
	s := &stubSurface{status: http.StatusForbidden,
		answer: `{"error":"adminapi: the token carries no admin role"}`}
	sess, _ := sessionOn(t, s, "tok-plain")

	_, err := sess.Admin().People(context.Background())
	var ref *Refusal
	if !errors.As(err, &ref) {
		t.Fatalf("the refusal did not come back as one: %v", err)
	}
	if !ref.Denied() {
		t.Errorf("403 does not read as a standing refusal: %+v", ref)
	}
	if !strings.Contains(ref.Msg, "no admin role") {
		t.Errorf("the surface's words were replaced by %q", ref.Msg)
	}

	// A fault is not a standing refusal, and must not read as one.
	s.status, s.answer = http.StatusInternalServerError, `{"error":"store: unreachable"}`
	_, err = sess.Admin().People(context.Background())
	if !errors.As(err, &ref) || ref.Denied() {
		t.Errorf("a fault reads as a standing refusal: %v", err)
	}

	// And a rule the surface holds is a third thing: the person had every
	// standing, the call was well formed, and the answer is about the state
	// it would have left behind.
	s.status = http.StatusConflict
	s.answer = `{"error":"the last administrator stays — add another administrator first"}`
	err = sess.Admin().Disable(context.Background(), "owner")
	if !errors.As(err, &ref) {
		t.Fatalf("the refusal did not come back as one: %v", err)
	}
	if !ref.Rule() || ref.Denied() {
		t.Errorf("a held rule reads as %+v", ref)
	}
	if !strings.Contains(ref.Msg, "the last administrator stays") {
		t.Errorf("the surface's words were replaced by %q", ref.Msg)
	}
}

// The surface says which person it is holding, so a screen never has to
// work out for itself which group administers somebody else's deployment.
func TestTheSurfaceNamesTheLastAdministrator(t *testing.T) {
	s := &stubSurface{status: http.StatusOK, answer: `[
		{"id":"u-1","username":"owner","status":"active","groups":["admin"],
		 "credentials":1,"last_admin":true},
		{"id":"u-2","username":"avery","status":"active","groups":["admin"],
		 "credentials":1}]`}
	sess, _ := sessionOn(t, s, "tok-abc")

	people, err := sess.Admin().People(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !people[0].LastAdmin {
		t.Errorf("the person the surface is holding did not arrive marked: %+v", people[0])
	}
	if people[1].LastAdmin {
		t.Errorf("a person the surface said nothing about arrived marked: %+v", people[1])
	}
}

// A deployment that declares no administration surface has none: the
// support layer says so, and a session offers nothing to reach it with.
func TestNoDeclarationMeansNoSurface(t *testing.T) {
	sp := &Support{cfg: Config{}}
	if sp.AdminSurface() != "" {
		t.Errorf("an undeclared surface reads as %q", sp.AdminSurface())
	}
	sess := &Session{Persona: "owner", sp: sp, bearer: "tok-abc"}
	if sess.Admin() != nil {
		t.Error("a session reaches an administration surface the deployment does not run")
	}
}

// The rest of the published contract, each call measured for the wire
// shape the surface actually reads: the path, the method, and the body
// spelled the way the API spells them.
func TestEveryAdminCallSpeaksTheSurfacesOwnShapes(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name               string
		call               func(a *Admin) error
		method, path, body string
	}{
		{"create", func(a *Admin) error {
			return a.Create(ctx, "avery", "Avery", []string{"realm"})
		}, http.MethodPost, "/api/admin/users",
			`{"display_name":"Avery","groups":["realm"],"username":"avery"}`},
		{"groups", func(a *Admin) error {
			return a.SetGroups(ctx, "avery", []string{"realm", "keeper"})
		}, http.MethodPost, "/api/admin/users/avery/groups",
			`{"groups":["realm","keeper"]}`},
		{"enable", func(a *Admin) error {
			return a.Enable(ctx, "avery")
		}, http.MethodPost, "/api/admin/users/avery/status", `{"status":"active"}`},
		{"client-add", func(a *Admin) error {
			return a.CreateClient(ctx, "kiosk", "Kiosk", []string{"http://k/cb"})
		}, http.MethodPost, "/api/admin/clients",
			`{"client_id":"kiosk","name":"Kiosk","redirect_uris":["http://k/cb"]}`},
		{"client-delete", func(a *Admin) error {
			return a.DeleteClient(ctx, "kiosk")
		}, http.MethodDelete, "/api/admin/clients/kiosk", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubSurface{status: http.StatusOK, answer: `{}`}
			sess, _ := sessionOn(t, stub, "brr_token")
			if err := tc.call(sess.Admin()); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if stub.bearer != "Bearer brr_token" {
				t.Errorf("%s rode %q, want the person's own bearer", tc.name, stub.bearer)
			}
			if stub.method != tc.method || stub.path != tc.path {
				t.Errorf("%s asked %s %s, want %s %s", tc.name,
					stub.method, stub.path, tc.method, tc.path)
			}
			if tc.body != "" && canonJSON(t, stub.body) != tc.body {
				t.Errorf("%s sent %s, want %s", tc.name, canonJSON(t, stub.body), tc.body)
			}
		})
	}
}

// canonJSON re-marshals with sorted keys so the wire assertion is about
// shape, not map order.
func canonJSON(t *testing.T, raw string) string {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("not JSON: %q", raw)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// The groups and the applications come back as the surface published them.
func TestGroupsAndClientsComeBackWhole(t *testing.T) {
	ctx := context.Background()
	stub := &stubSurface{status: http.StatusOK, answer: `["admin","realm"]`}
	sess, _ := sessionOn(t, stub, "brr")
	groups, err := sess.Admin().Groups(ctx)
	if err != nil || len(groups) != 2 || groups[0] != "admin" {
		t.Fatalf("groups = %v, %v", groups, err)
	}
	stub2 := &stubSurface{status: http.StatusOK,
		answer: `[{"client_id":"shell","name":"Shell","redirect_uris":["http://s/cb"]}]`}
	sess2, _ := sessionOn(t, stub2, "brr")
	clients, err := sess2.Admin().Clients(ctx)
	if err != nil || len(clients) != 1 || clients[0].ClientID != "shell" ||
		clients[0].RedirectURIs[0] != "http://s/cb" {
		t.Fatalf("clients = %v, %v", clients, err)
	}
}

// IsAdmin reads the person's own token, and only for the one purpose of
// drawing the key: the roles claim carries it or it does not, and a token
// that is not a token carries nothing.
func TestIsAdminReadsTheTokensOwnRoles(t *testing.T) {
	mint := func(roles string) string {
		payload := base64.RawURLEncoding.EncodeToString(
			[]byte(`{"sub":"u-1","roles":` + roles + `}`))
		return "eyJhbGciOiJSUzI1NiJ9." + payload + ".sig"
	}
	cases := []struct {
		name   string
		bearer string
		want   bool
	}{
		{"admin", mint(`["admin","realm"]`), true},
		{"plain", mint(`["realm"]`), false},
		{"no roles", mint(`[]`), false},
		{"opaque token", "sit_notajwt", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := &Session{bearer: tc.bearer}
			if got := sess.IsAdmin(); got != tc.want {
				t.Errorf("IsAdmin(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
