package auth

import (
	log "github.com/sirupsen/logrus"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/config"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/util"
	"net/http"
)

func OauthRedirectHandler(handler OAuthHandler) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		contextLogger := log.WithContext(ctx)
		envValues := config.NewEnvironmentConfig()
		err := r.ParseForm()
		if err != nil {
			http.Redirect(w, r, envValues.AuthErrorRedirectURL, http.StatusSeeOther)
			contextLogger.WithError(err).Error("could not parse incoming query")
			util.WithBodyAndStatus(nil, http.StatusBadRequest, w)
			return
		}

		code := r.FormValue("code")
		contextLogger.Infof("Auth code from xero: %v", code)
		_, err = handler.OAuthService(ctx, code)
		if err != nil {
			http.Redirect(w, r, envValues.AuthErrorRedirectURL, http.StatusSeeOther)
			contextLogger.WithError(err).Error("Failed to fetch the access token")
			util.WithBodyAndStatus("Failed to connect to Xero", http.StatusBadRequest, w)
			return
		}

		http.Redirect(w, r, envValues.AuthSuccessRedirectURL, http.StatusSeeOther)
		return
	}
}
