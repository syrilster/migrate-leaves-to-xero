package auth

import (
	"context"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/config"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/model"
	"net/http"
)

type OAuthHandler interface {
	OAuthService(ctx context.Context, code string) (*model.XeroResponse, error)
}

func Route(handler OAuthHandler) (route config.Route) {
	route = config.Route{
		Path:    "/oauth/redirect",
		Method:  http.MethodGet,
		Handler: OauthRedirectHandler(handler),
	}

	return route
}
