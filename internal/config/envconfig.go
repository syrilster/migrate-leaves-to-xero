package config

import (
	"os"
	"strconv"
)

type envConfig struct {
	LogLevel               string
	ServerPort             int
	Version                string
	BaseUrl                string
	XeroKey                string
	XeroSecret             string
	XeroEndpoint           string
	XeroAuthEndpoint       string
	XeroRedirectURI        string
	XlsFileLocation        string
	AuthSuccessRedirectURL string
	AuthErrorRedirectURL   string
	EmailTo                string
	EmailFrom              string
	AuthTokenFileLocation  string
	RateLimitTimeout       int
}

func NewEnvironmentConfig() *envConfig {
	return &envConfig{
		LogLevel:               getEnvString("LOG_LEVEL", "INFO"),
		ServerPort:             getEnvInt("SERVER_PORT", 0),
		Version:                getEnvString("VERSION", ""),
		BaseUrl:                "",
		XeroKey:                getEnvString("XERO_CLIENT_ID", ""),
		XeroSecret:             getEnvString("XERO_SECRET", ""),
		XeroEndpoint:           getEnvString("XERO_ENDPOINT", ""),
		XeroAuthEndpoint:       getEnvString("XERO_AUTH_ENDPOINT", ""),
		XeroRedirectURI:        getEnvString("XERO_REDIRECT_URI", ""),
		XlsFileLocation:        getEnvString("XLS_FILE_LOCATION", ""),
		AuthTokenFileLocation:  getEnvString("AUTH_TOKEN_FILE_LOCATION", ""),
		AuthSuccessRedirectURL: getEnvString("AUTH_SUCCESS_REDIRECT_URL", ""),
		AuthErrorRedirectURL:   getEnvString("AUTH_ERROR_REDIRECT_URL", ""),
		EmailTo:                getEnvString("EMAIL_TO", ""),
		EmailFrom:              getEnvString("EMAIL_FROM", ""),
		RateLimitTimeout:       getEnvInt("RATE_LIMIT_TIMEOUT", 1),
	}
}

// helper function to read an environment or return a default value
func getEnvString(key string, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return defaultVal
}

// helper function to read an environment or return a default value
func getEnvInt(key string, defaultVal int) int {
	val, err := strconv.Atoi(getEnvString(key, strconv.Itoa(defaultVal)))
	if err == nil {
		return val
	}

	return defaultVal
}
