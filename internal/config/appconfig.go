package config

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/xero"
	"net/http"
	_ "os"
	"time"

	"github.com/syrilster/migrate-leave-krow-to-xero/internal/customhttp"
)

type ApplicationConfig struct {
	envValues   *envConfig
	xeroClient  xero.ClientInterface
	emailClient *ses.SES
}

//Version returns application version
func (cfg *ApplicationConfig) Version() string {
	return cfg.envValues.Version
}

//ServerPort returns the port no to listen for requests
func (cfg *ApplicationConfig) ServerPort() int {
	return cfg.envValues.ServerPort
}

//BaseURL returns the base URL
func (cfg *ApplicationConfig) BaseURL() string {
	return cfg.envValues.BaseUrl
}

//XeroEndpoint returns the xero endpoint
func (cfg *ApplicationConfig) XeroEndpoint() xero.ClientInterface {
	return cfg.xeroClient
}

//XeroKey returns the xero client id
func (cfg *ApplicationConfig) XeroKey() string {
	return cfg.envValues.XeroKey
}

//XeroSecret returns the xero client secret
func (cfg *ApplicationConfig) XeroSecret() string {
	return cfg.envValues.XeroSecret
}

//XeroAuthEndpoint returns the auth related endpoint
func (cfg *ApplicationConfig) XeroAuthEndpoint() string {
	return cfg.envValues.XeroAuthEndpoint
}

//XeroRedirectURI returns the redirect URI
func (cfg *ApplicationConfig) XeroRedirectURI() string {
	return cfg.envValues.XeroRedirectURI
}

//XlsFileLocation returns the file location to read the leave requests
func (cfg *ApplicationConfig) XlsFileLocation() string {
	return cfg.envValues.XlsFileLocation
}

//EmailClient returns the ses client with config
func (cfg *ApplicationConfig) EmailClient() *ses.SES {
	return cfg.emailClient
}

//EmailTo returns the to email address
func (cfg *ApplicationConfig) EmailTo() string {
	return cfg.envValues.EmailTo
}

//EmailFrom returns the From email address
func (cfg *ApplicationConfig) EmailFrom() string {
	return cfg.envValues.EmailFrom
}

//AuthTokenFileLocation returns the temp loc to store auth file
func (cfg *ApplicationConfig) AuthTokenFileLocation() string {
	return cfg.envValues.AuthTokenFileLocation
}

//NewApplicationConfig loads config values from environment and initialises config
func NewApplicationConfig() *ApplicationConfig {
	envValues := NewEnvironmentConfig()
	httpCommand := NewHTTPCommand()
	xeroClient := xero.NewClient(envValues.XeroEndpoint, httpCommand, envValues.AuthTokenFileLocation)
	emailClient := ses.New(session.New(), aws.NewConfig().WithRegion("ap-southeast-2"))
	return &ApplicationConfig{
		envValues:   envValues,
		xeroClient:  xeroClient,
		emailClient: emailClient,
	}
}

// NewHTTPCommand returns the HTTP client
func NewHTTPCommand() customhttp.HTTPCommand {
	httpCommand := customhttp.New(
		customhttp.WithHTTPClient(&http.Client{Timeout: 5 * time.Second}),
	).Build()

	return httpCommand
}
