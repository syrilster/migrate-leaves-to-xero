package middlewares

import (
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/util"
	"net/http"
)

//RuntimeHealthCheck is a sample healt check func
func RuntimeHealthCheck() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		util.WithBodyAndStatus("All OK", http.StatusOK, w)
	}
}
