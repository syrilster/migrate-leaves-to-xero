package internal

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/auth"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/config"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/middlewares"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/xero"
	"net/http"
)

//StatusRoute health check route
func StatusRoute() (route config.Route) {
	route = config.Route{
		Path:    "/health",
		Method:  http.MethodGet,
		Handler: middlewares.RuntimeHealthCheck(),
	}
	return route
}

type ServerConfig interface {
	Version() string
	BaseURL() string
	XeroEndpoint() xero.ClientInterface
	XeroKey() string
	XeroSecret() string
	XeroAuthEndpoint() string
	XeroRedirectURI() string
	XlsFileLocation() string
	EmailClient() *ses.SES
	EmailTo() string
	EmailFrom() string
	AuthTokenFileLocation() string
}

func SetupServer(cfg ServerConfig) *config.Server {
	basePath := fmt.Sprintf("/%v", cfg.Version())
	service := NewService(cfg.XeroEndpoint(), cfg.XlsFileLocation(), cfg.EmailClient(), cfg.EmailTo(), cfg.EmailFrom())
	authService := auth.NewAuthService(cfg.XeroKey(), cfg.XeroSecret(), cfg.XeroAuthEndpoint(), cfg.XeroRedirectURI(), cfg.AuthTokenFileLocation())
	server := config.NewServer().
		WithRoutes(
			"", StatusRoute(),
		).
		WithRoutes(
			basePath,
			Route(service),
			auth.Route(authService),
		)
	return server
}
