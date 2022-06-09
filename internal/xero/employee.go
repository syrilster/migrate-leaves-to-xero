package xero

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/googleapis/gax-go/v2"
	log "github.com/sirupsen/logrus"
)

const (
	headerKeyXeroTenantID = "xero-tenant-id"
	headerKeyAuth         = "Authorization"
	bearer                = "Bearer"

	accessTokenFetchErr = "Error fetching the access token"
	empApiName          = "GetEmployees"
)

func (c *client) NewGetEmployeesRequest(ctx context.Context, tenantID string, page string) (*ReusableRequest, error) {
	contextLogger := log.WithContext(ctx)
	contextLogger.Info("Building new get employees request for tenant: ", tenantID)

	contextLogger.Info("Building Xero Employee Endpoint with page filter for page: ", page)
	req, err := http.NewRequest(http.MethodGet, buildXeroEmployeesEndpoint(c.URL, page), nil)
	if err != nil {
		contextLogger.WithError(err).Errorf("failed to build HTTP request")
		return nil, err
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		contextLogger.WithError(err).Errorf("Error fetching the access token")
		return nil, err
	}

	req.Header.Set(headerKeyAuth, fmt.Sprintf("%s %s", bearer, accessToken))
	req.Header.Set(headerKeyXeroTenantID, tenantID)

	return &ReusableRequest{
		Request: req,
	}, nil
}

func (c *client) GetEmployees(ctx context.Context, req *ReusableRequest) (*EmpResponse, error) {
	var d time.Duration

	retryCtx, cancel, backOff := newRetry(ctx)
	defer cancel()

	for {
		res, err := c.getEmployees(ctx, req.Request)
		if err != nil {
			if errors.Is(err, unauthorized) {
				return nil, err
			}

			if errors.Is(err, exceededRateLimit) {
				d = backOff.Pause()
			}

			if !errors.Is(err, nonRetryable) {
				if innerErr := gax.Sleep(retryCtx, d); innerErr != nil {
					return nil, errors.New(fmt.Sprint("failed, retry limit expired:", err))
				}
				continue
			}
			return nil, err
		}
		return res, nil
	}
}

func (c *client) getEmployees(ctx context.Context, req *http.Request) (*EmpResponse, error) {
	contextLogger := log.WithContext(ctx)
	res, err := c.Do(req)
	if err != nil {
		return nil, errors.New(fmt.Sprint(fmt.Sprintf("failed to execute %s request ", empApiName), err))
	}

	err = getHTTPStatusCode(ctx, res, empApiName)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		contextLogger.WithError(err).Errorf("error reading xero API data resp body (%s)", body)
		return nil, err
	}

	defer func() {
		if err = res.Body.Close(); err != nil {
			fmt.Println("Error when closing:", err)
		}
	}()

	response := &EmpResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		contextLogger.WithError(err).Errorf("there was an error un marshalling the xero API resp. %v", err)
		return nil, err
	}

	return response, nil
}

func (c *client) EmployeeLeaveBalance(ctx context.Context, tenantID string, empID string) (*LeaveBalanceResponse, error) {
	contextLogger := log.WithContext(ctx)
	contextLogger.Info("Fetching leave balance for employee: ", empID)
	httpRequest, err := http.NewRequest(http.MethodGet, c.buildXeroLeaveBalanceEndpoint(empID), nil)
	if err != nil {
		return nil, err
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		contextLogger.WithError(err).Errorf(accessTokenFetchErr)
		return nil, err
	}
	httpRequest.Header.Set(headerKeyAuth, fmt.Sprintf("%s %s", bearer, accessToken))
	httpRequest.Header.Set(headerKeyXeroTenantID, tenantID)

	resp, err := c.Client.Do(httpRequest)
	if err != nil {
		contextLogger.WithError(err).Errorf("there was an error calling the xero connection API. %v", err)
		return nil, err
	}

	defer func() {
		if err = resp.Body.Close(); err != nil {
			contextLogger.WithError(err).Errorf("Error closing the ioReader. %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		contextLogger.Infof("status returned from xero service %s ", resp.Status)
		return nil, fmt.Errorf("xero service (EmployeeLeaveBalance) returned status: %s ", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		contextLogger.WithError(err).Errorf("error reading xero API data resp body (%s)", body)
		return nil, err
	}

	response := &LeaveBalanceResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		contextLogger.WithError(err).Errorf("there was an error un marshalling the xero API resp. %v", err)
		return nil, err
	}

	response.RateLimitRemaining, err = strconv.Atoi(resp.Header.Get("X-MinLimit-Remaining"))
	if err != nil {
		contextLogger.WithError(err).Errorf("there was an error un marshalling the xero API resp headers. %v", err)
		return nil, err
	}

	return response, nil
}

func (c *client) EmployeeLeaveApplication(ctx context.Context, tenantID string, request LeaveApplicationRequest) error {
	contextLogger := log.WithContext(ctx)
	var req = make([]LeaveApplicationRequest, 1)
	req[0] = request
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpRequest, err := http.NewRequest(http.MethodPost, c.buildXeroLeaveApplicationEndpoint(), bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		contextLogger.WithError(err).Errorf(accessTokenFetchErr)
		return err
	}

	httpRequest.Header.Set(headerKeyAuth, fmt.Sprintf("%s %s", bearer, accessToken))
	httpRequest.Header.Set(headerKeyXeroTenantID, tenantID)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	resp, err := c.Client.Do(httpRequest)
	if err != nil {
		contextLogger.WithError(err).Errorf("there was an error calling the xero connection API. %v", err)
		return err
	}

	defer func() {
		if err = resp.Body.Close(); err != nil {
			contextLogger.WithError(err).Errorf("Error closing the ioReader. %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		contextLogger.Infof("status returned from xero service %s ", resp.Status)
		return fmt.Errorf("xero service (EmployeeLeaveApplication) returned status: %s ", resp.Status)
	}

	return nil
}

func (c *client) buildXeroLeaveBalanceEndpoint(empID string) string {
	return c.URL + "/payroll.xro/1.0/Employees/" + empID
}

func (c *client) buildXeroLeaveApplicationEndpoint() string {
	return c.URL + "/payroll.xro/1.0/LeaveApplications"
}
