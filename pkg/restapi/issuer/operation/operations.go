/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	"github.com/trustbloc/edge-sandbox/pkg/internal/common/support"
)

const (
	login    = "/login"
	callback = "/callback"

	oauthCookieName = "oauthstate"
	stateFormKey    = "state"
	codeFormKey     = "code"

	defaultCookieExpiry = 20
	stateValueLength    = 16 // minutes
)

// Handler http handler for each controller API endpoint
type Handler interface {
	Path() string
	Method() string
	Handle() http.HandlerFunc
}

// Operation defines handlers for authorization service
type Operation struct {
	handlers  []Handler
	cmsConfig *oauth2.Config
}

// Config defines configuration for issuer operations
type Config struct {
	OAuth2Config *oauth2.Config
}

// New returns authorization instance
func New(config *Config) *Operation {
	svc := &Operation{cmsConfig: config.OAuth2Config}
	svc.registerHandler()

	return svc
}

// Login using oauth2, will redirect to Auth Code URL
func (c *Operation) Login(w http.ResponseWriter, r *http.Request) {
	// AuthCodeURL receives state that is a token to protect the user from CSRF attacks
	oauthState := generateStateOauthCookie(w)

	u := c.cmsConfig.AuthCodeURL(oauthState)
	http.Redirect(w, r, u, http.StatusTemporaryRedirect)
}

// Callback for oauth2 login
func (c *Operation) Callback(w http.ResponseWriter, r *http.Request) {
	// read oauthState from cookie
	oauthState, err := r.Cookie(oauthCookieName)
	if err != nil {
		log.Error(err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)

		return
	}

	// validate that oauth cookie state matches the the state query parameter on your redirect callback
	if r.FormValue(stateFormKey) != oauthState.Value {
		log.Warn("invalid oauth state")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)

		return
	}

	// exchange code for token
	_, err = c.cmsConfig.Exchange(context.Background(), r.FormValue(codeFormKey))
	if err != nil {
		log.Error(err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)

		return
	}

	// get user data from CMS here (display token for now)
	c.writeResponse(w, validCredential)
}

func generateStateOauthCookie(w http.ResponseWriter) string {
	// generate random bytes for state value
	b := make([]byte, stateValueLength)

	_, err := rand.Read(b)
	if err != nil {
		log.Error(err)
	}

	state := base64.URLEncoding.EncodeToString(b)
	cookie := http.Cookie{Name: oauthCookieName,
		Value:   state,
		Expires: time.Now().Add(defaultCookieExpiry * time.Minute)}

	http.SetCookie(w, &cookie)

	return state
}

// registerHandler register handlers to be exposed from this service as REST API endpoints
func (c *Operation) registerHandler() {
	// Add more protocol endpoints here to expose them as controller API endpoints
	c.handlers = []Handler{
		support.NewHTTPHandler(login, http.MethodGet, c.Login),
		support.NewHTTPHandler(callback, http.MethodGet, c.Callback),
	}
}

// writeResponse writes interface value to response
func (c *Operation) writeResponse(rw io.Writer, v interface{}) {
	err := json.NewEncoder(rw).Encode(v)
	// as of now, just log errors for writing response
	if err != nil {
		log.Errorf("Unable to send error response, %s", err)
	}
}

// GetRESTHandlers get all controller API handler available for this service
func (c *Operation) GetRESTHandlers() []Handler {
	return c.handlers
}

//nolint:lll
const validCredential = `{
  "@context": "https://www.w3.org/2018/credentials/v1",
  "id": "http://example.edu/credentials/1872",
  "type": "VerifiableCredential",
  "credentialSubject": {
    "id": "did:example:ebfeb1f712ebc6f1c276e12ec21"
  },
  "issuer": {
    "id": "did:example:76e12ec712ebc6f1c221ebfeb1f",
    "name": "Example University"
  },
  "issuanceDate": "2010-01-01T19:23:24Z",
  "proof": {
    "type": "RsaSignature2018",
    "created": "2018-06-18T21:19:10Z",
    "proofPurpose": "assertionMethod",
    "verificationMethod": "https://example.com/jdoe/keys/1",
    "jws": "eyJhbGciOiJQUzI1NiIsImI2NCI6ZmFsc2UsImNyaXQiOlsiYjY0Il19..DJBMvvFAIC00nSGB6Tn0XKbbF9XrsaJZREWvR2aONYTQQxnyXirtXnlewJMBBn2h9hfcGZrvnC1b6PgWmukzFJ1IiH1dWgnDIS81BH-IxXnPkbuYDeySorc4QU9MJxdVkY5EL4HYbcIfwKj6X4LBQ2_ZHZIu1jdqLcRZqHcsDF5KKylKc1THn5VRWy5WhYg_gBnyWny8E6Qkrze53MR7OuAmmNJ1m1nN8SxDrG6a08L78J0-Fbas5OjAQz3c17GY8mVuDPOBIOVjMEghBlgl3nOi1ysxbRGhHLEK4s0KKbeRogZdgt1DkQxDFxxn41QWDw_mmMCjs9qxg0zcZzqEJw"
  },
  "expirationDate": "2020-01-01T19:23:24Z",
  "credentialStatus": {
    "id": "https://example.edu/status/24",
    "type": "CredentialStatusList2017"
  }
}
`
