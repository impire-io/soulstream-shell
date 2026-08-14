package rig

// A bearer of this deployment's own issuing.
//
// The machine half of the administration surface takes one thing: an
// access token the sign-in surface signed, whose groups say the holder
// administers. A signed-in person's session holds one of those and hands
// it to nobody — rightly — so a test that means to ask the sign-in
// surface directly, the way any other client would, has to earn its own.
//
// This is that ceremony walked from outside the product: a relying party
// the deployment has never seen registers itself (the surface takes its
// own dynamic registration), signs the named person in with their
// passkey, and exchanges the code. Nothing here goes around the door. It
// is the door, walked by a program.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"github.com/impire-io/soulstream-idp/authtest"
)

// rpCallback is where the scripted browser stops. Nothing listens on it,
// so the walk reads the code off the redirect it is handed rather than
// following it anywhere.
const rpCallback = "http://127.0.0.1:1/cb"

// Bearer signs the named person in as a relying party of its own and
// returns the access token the sign-in surface issued them — what that
// surface's machine half admits on, and nothing this product minted.
func (r *Rig) Bearer(auth *authtest.Authenticator, username string) (string, error) {
	issuer := r.Issuer
	var doc struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		RegistrationEndpoint  string `json:"registration_endpoint"`
	}
	if err := getJSON(issuer+"/.well-known/openid-configuration", &doc); err != nil {
		return "", err
	}
	if doc.RegistrationEndpoint == "" {
		return "", errors.New("rig: the sign-in surface registers no clients of its own — " +
			"there is no way for a test to hold a bearer it issued")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}
	callback, err := url.Parse(rpCallback)
	if err != nil {
		return "", err
	}
	cl := &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		if req.URL.Host == callback.Host {
			return http.ErrUseLastResponse
		}
		return nil
	}}

	var reg struct {
		ClientID string `json:"client_id"`
	}
	body := fmt.Sprintf(`{"client_name":"gate probe","redirect_uris":[%q]}`, rpCallback)
	if err := postForJSON(cl, doc.RegistrationEndpoint, issuer, []byte(body), &reg); err != nil {
		return "", fmt.Errorf("rig: register a client: %w", err)
	}

	// PKCE: this client keeps no secret, so the proof is the verifier it
	// never sent until the exchange.
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(seed)
	sum := sha256.Sum256([]byte(verifier))

	q := url.Values{
		"client_id": {reg.ClientID}, "redirect_uri": {rpCallback},
		"response_type": {"code"}, "scope": {"openid"}, "state": {"gate"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
	}
	resp, err := cl.Get(doc.AuthorizationEndpoint + "?" + q.Encode())
	if err != nil {
		return "", fmt.Errorf("rig: authorize: %w", err)
	}
	page, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	authReqID := resp.Request.URL.Query().Get("authRequestID")
	if authReqID == "" {
		return "", fmt.Errorf("rig: no authRequestID; the walk landed on %s", resp.Request.URL)
	}
	m := csrfRe.FindSubmatch(page)
	if m == nil {
		return "", errors.New("rig: no csrf field on the sign-in page")
	}

	ceremony := url.Values{"authRequestID": {authReqID}, "csrf": {string(m[1])},
		"username": {username}}
	raw, err := postJSON(cl, issuer+"/login/begin?"+ceremony.Encode(), issuer, nil)
	if err != nil {
		return "", err
	}
	var begin beginResp
	if err := json.Unmarshal(raw, &begin); err != nil {
		return "", fmt.Errorf("rig: login begin: %w", err)
	}
	cred, err := auth.GetResponse(begin.Options)
	if err != nil {
		return "", fmt.Errorf("rig: authenticator: %w", err)
	}
	ceremony.Set("ceremonyID", begin.CeremonyID)
	raw, err = postJSON(cl, issuer+"/login/finish?"+ceremony.Encode(), issuer, cred)
	if err != nil {
		return "", err
	}
	var fin struct {
		Redirect string `json:"redirect"`
	}
	if err := json.Unmarshal(raw, &fin); err != nil {
		return "", fmt.Errorf("rig: login finish: %w", err)
	}
	redirect := fin.Redirect
	if strings.HasPrefix(redirect, "/") {
		redirect = issuer + redirect
	}
	handed, err := cl.Get(redirect)
	if err != nil {
		return "", fmt.Errorf("rig: callback: %w", err)
	}
	_, _ = io.Copy(io.Discard, handed.Body)
	_ = handed.Body.Close()
	landing, err := url.Parse(handed.Header.Get("Location"))
	if err != nil || landing.Query().Get("code") == "" {
		return "", fmt.Errorf("rig: the ceremony handed back no code: %s %s",
			handed.Status, handed.Header.Get("Location"))
	}

	exchange := url.Values{
		"grant_type": {"authorization_code"}, "code": {landing.Query().Get("code")},
		"redirect_uri": {rpCallback}, "client_id": {reg.ClientID},
		"code_verifier": {verifier},
	}
	tokenResp, err := cl.PostForm(doc.TokenEndpoint, exchange)
	if err != nil {
		return "", fmt.Errorf("rig: token exchange: %w", err)
	}
	defer func() { _ = tokenResp.Body.Close() }()
	out, _ := io.ReadAll(tokenResp.Body)
	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("rig: token exchange: %d: %s", tokenResp.StatusCode, out)
	}
	var issued struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(out, &issued); err != nil || issued.AccessToken == "" {
		return "", fmt.Errorf("rig: no access token in %s", out)
	}
	return issued.AccessToken, nil
}

// Ask carries one call to the sign-in surface's machine half as the
// holder of a bearer, and hands back what it answered — the status and
// the body, both, because on this surface the refusal is the point as
// often as the answer is.
func (r *Rig) Ask(bearer, method, path string, body any) (int, string, error) {
	if r.AdminBase == "" {
		return 0, "", errors.New("rig: this deployment administers its sign-ins elsewhere")
	}
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, "", err
		}
		rdr = strings.NewReader(string(raw))
	}
	req, err := http.NewRequest(method, r.AdminBase+path, rdr)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	said, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(said), nil
}

func getJSON(u string, out any) error {
	resp, err := http.Get(u)
	if err != nil {
		return fmt.Errorf("GET %s: %w", u, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %d: %s", u, resp.StatusCode, raw)
	}
	return json.Unmarshal(raw, out)
}

// postForJSON is postJSON's sibling for the calls that answer 201.
func postForJSON(cl *http.Client, u, origin string, body []byte, out any) error {
	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	resp, err := cl.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", u, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("POST %s: %d: %s", u, resp.StatusCode, raw)
	}
	return json.Unmarshal(raw, out)
}
