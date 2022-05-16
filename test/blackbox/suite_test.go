package blackbox

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/suite"
)

var (
	connectionResp = `[
    {
        "id": "909f5356-c509-4dc2-bee2-f67ef9703bc8",
        "authEventId": "228dd1d3-59e1-4d89-88e7-efca52fcbbc1",
        "tenantId": "2e9e4e41-feab-4bb2-9fc1-ef1c61fd7e9b",
        "tenantType": "ORGANISATION",
        "tenantName": "Kasna",
        "createdDateUtc": "2022-04-14T04:05:18.2318600",
        "updatedDateUtc": "2022-04-14T04:05:18.2331860"
    },
]`
)

// entrypoint for test
func TestApiSuite(t *testing.T) {
	suite.Run(t, new(apiSuite))
}

type apiSuite struct {
	suite.Suite

	ctx context.Context
}

func (a *apiSuite) SetupSuite() {
	// block all HTTP requests
	httpmock.Activate()
}

func (a *apiSuite) TearDownTest() {
	// remove any mocks after each test
	httpmock.Reset()
}

func (a *apiSuite) TearDownSuite() {
	httpmock.DeactivateAndReset()
}

func (a *apiSuite) Test_BasicSuccess() {
	fmt.Println("INSIDE BB TEST SUITE ============================================")
	httpmock.RegisterResponder(http.MethodGet, "https://api.test.xero.com/connections",
		httpmock.NewStringResponder(http.StatusOK, connectionResp))
}
