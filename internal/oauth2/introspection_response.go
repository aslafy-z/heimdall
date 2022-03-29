package oauth2

import (
	"errors"

	"github.com/dadrus/heimdall/internal/pipeline/handler/authenticators"
	"gopkg.in/square/go-jose.v2/jwt"
)

type IntrospectionResponse struct {
	jwt.Claims
	Active    bool   `json:"active"`
	Scopes    Scopes `json:"scope"`
	ClientId  string `json:"client_id"`
	Username  string `json:"username"`
	TokenType string `json:"token_type"`
}

func (ir *IntrospectionResponse) Verify(asserter authenticators.ClaimAsserter) error {
	if !ir.Active {
		return errors.New("token is not active")
	}

	return nil
}
