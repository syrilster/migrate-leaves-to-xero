package auth

import (
	"context"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/model"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

const filePerm = 0600

type Service struct {
	xeroKey          string
	xeroSecret       string
	xeroAuthEndpoint string
	xeroRedirectURI  string
	AuthTokenFileLoc string
}

func NewAuthService(key string, secret string, authURL string, redirectURI string, authFileLoc string) *Service {
	return &Service{
		xeroKey:          key,
		xeroSecret:       secret,
		xeroAuthEndpoint: authURL,
		xeroRedirectURI:  redirectURI,
		AuthTokenFileLoc: authFileLoc,
	}
}

func (service Service) OAuthService(ctx context.Context, code string) (*model.XeroResponse, error) {
	ctxLogger := log.WithContext(ctx)
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", service.xeroRedirectURI)

	req, err := http.NewRequest(http.MethodPost, service.xeroAuthEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		ctxLogger.WithError(err).Error("could not create HTTP request")
		return nil, err
	}
	req.SetBasicAuth(service.xeroKey, service.xeroSecret)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("accept", "application/json")

	// Send out the HTTP request
	httpClient := http.Client{}
	res, err := httpClient.Do(req)
	if err != nil {
		ctxLogger.WithError(err).Error("could not send HTTP request")
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		ctxLogger.Infof("status returned from xero Auth service is %s", res.Status)
		return nil, fmt.Errorf("xero auth service returned status: %s", res.Status)
	}

	// Parse the request body into the `XeroResponse` struct
	var resp *model.XeroResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		ctxLogger.WithError(err).Error("could not parse JSON response")
		return nil, err
	}

	file, err := json.MarshalIndent(resp, "", " ")
	if err != nil {
		ctxLogger.WithError(err).Error("Error preparing the json to write to file")
		return nil, err
	}

	err = ioutil.WriteFile(service.AuthTokenFileLoc, file, filePerm)
	if err != nil {
		ctxLogger.WithError(err).Error("Error writing token to file")
		return nil, err
	}
	return resp, nil
}
