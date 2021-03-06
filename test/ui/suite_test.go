package ui

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	defaultHost = "leave-migration-ui:3000"
)

// entrypoint for test
func TestApiSuite(t *testing.T) {
	suite.Run(t, new(apiSuite))
}

type apiSuite struct {
	suite.Suite

	httpClient *http.Client
	host       string
}

func (a *apiSuite) SetupSuite() {
	a.httpClient = &http.Client{
		Timeout: 2 * time.Minute,
	}

	a.host = defaultHost
}

// Test_BasicHealthCheck is a test to verify that the UI service is able to serve correctly
// without any errors. This is useful as a health check when updating to newer version of node and related dependencies.
func (a *apiSuite) Test_BasicHealthCheck() {
	url := fmt.Sprintf("http://%s", a.host)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(a.T(), err)

	r, err := a.httpClient.Do(req)
	require.NoError(a.T(), err)

	a.Require().Equal(http.StatusOK, r.StatusCode)
}
