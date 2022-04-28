package internal

import (
	"context"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/config"
	"net/http"
)

type XeroAPIHandler interface {
	MigrateLeaveKrowToXero(ctx context.Context) []string
}

func Route(xeroHandler XeroAPIHandler) (route config.Route) {
	route = config.Route{
		Path:    "/migrateLeaves",
		Method:  http.MethodPost,
		Handler: Handler(xeroHandler),
	}

	return route
}
