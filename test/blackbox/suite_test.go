package blackbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	defaultHost     = "blackboxapi:8000"
	mountebankHost  = "mountebank:2525"
	bbTestFilesPath = "/app/test/blackbox/testfiles"
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

type APIResponse struct {
	result []string
}

type MountebankResponse struct {
	Imposter []Imposter `json:"imposters"`
}

type Imposter struct {
	NumRequest int `json:"numberOfRequests"`
}

func (a *apiSuite) SetupSuite() {
	a.httpClient = &http.Client{
		//Transport: &http2.Transport{
		//	AllowHTTP: true,
		//	//DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
		//	//	return net.Dial(network, addr)
		//	//},
		//},
		// High timeout to support the retry tests as it takes pause due to 429 error
		Timeout: 2 * time.Minute,
	}

	a.host = defaultHost
}

func (a *apiSuite) Test_BasicSuccess() {
	url := fmt.Sprintf("http://%s/v1/migrateLeaves", a.host)
	resp := &APIResponse{}
	req := a.newFileUploadRequest(url, fmt.Sprintf("%s/digio_leave.xlsx", bbTestFilesPath))
	code := doHTTPRequest[APIResponse](a.T(), a.httpClient, req, resp, false)

	a.Require().Equal(http.StatusOK, code)
}

// Test_Success_RateLimitErrorRetryThenSuccess is a test to verify the rate limit scenario.
// The Mountebank stub data is set up in a way that the payroll endpoint response will
// return a 429 error for the first call and the subsequent call will work successfully.
// The test verifies that the total count of HTTP requests is as expected.
func (a *apiSuite) Test_Success_RateLimitErrorRetryThenSuccess() {
	// Need to get the initial request count as mountebank does not have an endpoint to reset number of requests.
	initial := getHTTPRequestCount(a.T(), a.httpClient)

	url := fmt.Sprintf("http://%s/v1/migrateLeaves", a.host)
	resp := &APIResponse{}
	req := a.newFileUploadRequest(url, fmt.Sprintf("%s/cmd_leave.xlsx", bbTestFilesPath))
	code := doHTTPRequest[APIResponse](a.T(), a.httpClient, req, resp, false)

	a.Require().Equal(http.StatusOK, code)

	latest := getHTTPRequestCount(a.T(), a.httpClient)
	// 7 Requests because 1 * connections, 2 * Employee, 2 * PayRollCalendars, 2 * Employees/{empID}, 1 * leaveApplication
	a.Require().Equal(8, latest-initial)
}

func (a *apiSuite) Test_ErrorScenario() {
	url := fmt.Sprintf("http://%s/v1/migrateLeaves", a.host)
	resp := &APIResponse{}
	req := a.newFileUploadRequest(url, fmt.Sprintf("%s/test_leave.xlsx", bbTestFilesPath))
	code := doHTTPRequest[[]string](a.T(), a.httpClient, req, &resp.result, true)
	r := resp.result

	a.Require().Equal(http.StatusInternalServerError, code)
	a.Require().NotEmpty(r)
	a.Require().Len(r, 2)

	a.Require().Contains(r, "Failed to get Organization details from Xero. Organization: Kasna. ")
	a.Require().Contains(r, "Employee not found in Xero. Employee: Test Data. Organization: DigIO  ")
}

func getHTTPRequestCount(t *testing.T, httpClient *http.Client) int {
	url := fmt.Sprintf("http://%s/imposters", mountebankHost)
	mbResp := &MountebankResponse{}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	code := doHTTPRequest[MountebankResponse](t, httpClient, req, mbResp, true)
	require.Equal(t, http.StatusOK, code)

	require.NotNil(t, mbResp)
	r := mbResp.Imposter[0]

	return r.NumRequest
}

// doHTTPRequest is a generic method to make an HTTP call with a given request
// and parse the response to the provided resp via generics.
func doHTTPRequest[T interface{}](t *testing.T, httpClient *http.Client, req *http.Request, res *T, readBody bool) int {
	r, err := httpClient.Do(req)
	require.NoError(t, err)

	if !readBody {
		return r.StatusCode
	}

	defer func() {
		require.NoError(t, r.Body.Close())
	}()

	b, err := ioutil.ReadAll(r.Body)
	require.NoError(t, err)

	require.NoError(t, json.Unmarshal(b, &res))

	return r.StatusCode
}

// newFileUploadRequest creates a new file upload http request
func (a *apiSuite) newFileUploadRequest(url, path string) *http.Request {
	file, err := os.Open(path)
	a.Require().NoError(err)

	defer func() {
		a.Require().NoError(file.Close())
	}()

	fi, err := file.Stat()
	a.Require().NoError(err)

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", fi.Name())
	a.Require().NoError(err)

	_, err = io.Copy(part, file)
	a.Require().NoError(err)

	err = writer.Close()
	a.Require().NoError(err)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body.Bytes()))
	a.Require().NoError(err)

	req.Header.Add("Content-Type", writer.FormDataContentType())

	return req
}
