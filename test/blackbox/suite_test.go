package blackbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/suite"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"testing"
)

const (
	defaultHost = "blackboxapi:8000"
)

// entrypoint for test
func TestApiSuite(t *testing.T) {
	suite.Run(t, new(apiSuite))
}

type apiSuite struct {
	suite.Suite

	ctx        context.Context
	httpClient *http.Client
	host       string
}

type APIResponse struct {
	res []string
}

func (a *apiSuite) SetupSuite() {
	a.httpClient = &http.Client{
		//Transport: &http2.Transport{
		//	AllowHTTP: true,
		//	//DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
		//	//	return net.Dial(network, addr)
		//	//},
		//},
		//Timeout: 45 * time.Second,
	}

	a.host = defaultHost
}

func (a *apiSuite) Test_BasicSuccess() {
	url := fmt.Sprintf("http://%s/v1/migrateLeaves", a.host)
	res := &APIResponse{}
	req := a.newFileUploadRequest(url, "file", "app/test/blackbox/digio_leave.xlsx")
	code, err := a.doHTTPRequest(req, res)
	a.Require().NoError(err)
	a.Require().Equal(http.StatusOK, code)
}

func (a *apiSuite) doHTTPRequest(req *http.Request, response *APIResponse) (int, error) {
	r, err := a.httpClient.Do(req)
	if err != nil {
		return 0, err
	}

	if r.StatusCode != http.StatusOK {
		return r.StatusCode, nil
	}

	defer func() {
		a.Require().NoError(r.Body.Close())
	}()

	b, err := ioutil.ReadAll(r.Body)
	a.Require().NoError(err)

	var resp string
	a.Require().NoError(json.Unmarshal(b, &resp))

	//response.res = resp
	return r.StatusCode, nil
}

// Creates a new file upload http request
func (a *apiSuite) newFileUploadRequest(url, paramName, path string) *http.Request {
	file, err := os.Open(path)
	a.Require().NoError(err)

	defer func() {
		a.Require().NoError(file.Close())
	}()

	fi, err := file.Stat()
	a.Require().NoError(err)

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(paramName, fi.Name())
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
