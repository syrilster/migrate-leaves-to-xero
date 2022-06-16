package xero

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/googleapis/gax-go/v2"
	log "github.com/sirupsen/logrus"
)

const (
	headerKeyXeroTenantID = "xero-tenant-id"
	headerKeyAuth         = "Authorization"
	bearer                = "Bearer"
	accessTokenFetchErr   = "Error fetching the access token"

	empApiName                 = "GetEmployees"
	empLeaveBalanceApiName     = "GetEmployeeLeaveBalance"
	empLeaveApplicationApiName = "EmployeeLeaveApplication"
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
		contextLogger.WithError(err).Errorf(accessTokenFetchErr)
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

	retryCtx, cancel, backOff := newRetry(ctx, c.RateLimitBackoff, c.RateLimitTimeout)
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
					return nil, fmt.Errorf("failed, retry limit expired: %v", err)
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
		return nil, fmt.Errorf("failed to execute %s request. Cause %v, %w", empApiName, err, nonRetryable)
	}

	err = getHTTPStatusCode(ctx, res, empApiName)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		contextLogger.WithError(err).Errorf("error reading GetEmployees resp body (%s)", body)
		return nil, fmt.Errorf("error reading GetEmployees resp body. cause: %v %w", err, nonRetryable)
	}

	defer func() {
		if err = res.Body.Close(); err != nil {
			fmt.Println("Error when closing:", err)
		}
	}()

	response := &EmpResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		contextLogger.WithError(err).Errorf("there was an error un marshalling the GetEmployees resp. %v", err)
		return nil, fmt.Errorf("there was an error un marshalling the GetEmployees resp. cause: %v %w", err, nonRetryable)
	}

	return response, nil
}

func (c *client) NewEmployeeLeaveBalanceRequest(ctx context.Context, tenantID string, empID string) (*ReusableRequest, error) {
	contextLogger := log.WithContext(ctx)
	contextLogger.Info("Fetching leave balance for employee: ", empID)

	req, err := http.NewRequest(http.MethodGet, buildXeroLeaveBalanceEndpoint(c.URL, empID), nil)
	if err != nil {
		contextLogger.WithError(err).Errorf("failed to build HTTP request")
		return nil, err
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		contextLogger.WithError(err).Errorf(accessTokenFetchErr)
		return nil, err
	}

	req.Header.Set(headerKeyAuth, fmt.Sprintf("%s %s", bearer, accessToken))
	req.Header.Set(headerKeyXeroTenantID, tenantID)

	return &ReusableRequest{
		Request: req,
	}, nil
}

func (c *client) EmployeeLeaveBalance(ctx context.Context, req *ReusableRequest) (*LeaveBalanceResponse, error) {
	var d time.Duration

	retryCtx, cancel, backOff := newRetry(ctx, c.RateLimitBackoff, c.RateLimitTimeout)
	defer cancel()

	for {
		res, err := c.employeeLeaveBalance(ctx, req.Request)
		if err != nil {
			if errors.Is(err, unauthorized) {
				return nil, err
			}

			if errors.Is(err, exceededRateLimit) {
				d = backOff.Pause()
			}

			if !errors.Is(err, nonRetryable) {
				if innerErr := gax.Sleep(retryCtx, d); innerErr != nil {
					return nil, fmt.Errorf("failed, retry limit expired: %v", err)
				}
				continue
			}
			return nil, err
		}
		return res, nil
	}
}

func (c *client) employeeLeaveBalance(ctx context.Context, req *http.Request) (*LeaveBalanceResponse, error) {
	contextLogger := log.WithContext(ctx)
	res, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute %s request. Cause %v, %w", empLeaveBalanceApiName, err, nonRetryable)
	}

	err = getHTTPStatusCode(ctx, res, empLeaveBalanceApiName)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		contextLogger.WithError(err).Errorf("error reading GetEmployeeLeaveBalance resp body (%s)", body)
		return nil, fmt.Errorf("error reading GetEmployeeLeaveBalance resp body. cause: %v %w", err, nonRetryable)
	}

	defer func() {
		if err = res.Body.Close(); err != nil {
			fmt.Println("Error when closing:", err)
		}
	}()

	response := &LeaveBalanceResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		contextLogger.WithError(err).Errorf("there was an error un marshalling the GetEmployeeLeaveBalance resp. %v", err)
		return nil, fmt.Errorf("there was an error un marshalling the GetEmployeeLeaveBalance resp. cause: %v %w", err, nonRetryable)
	}

	return response, nil
}

func (c *client) NewEmployeeLeaveApplicationRequest(ctx context.Context, tenantID string, leaveReq LeaveApplicationRequest) (*ReusableRequest, error) {
	contextLogger := log.WithContext(ctx)
	contextLogger.Info("Building new EmployeeLeaveApplication request for tenant: ", tenantID)

	r := make([]LeaveApplicationRequest, 1)
	r[0] = leaveReq
	payload, err := json.Marshal(r)
	if err != nil {
		contextLogger.WithError(err).Errorf("error marshalling the leave request")
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, buildXeroLeaveApplicationEndpoint(c.URL), bytes.NewBuffer(payload))
	if err != nil {
		contextLogger.WithError(err).Errorf("failed to build HTTP request")
		return nil, err
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		contextLogger.WithError(err).Errorf(accessTokenFetchErr)
		return nil, err
	}

	req.Header.Set(headerKeyAuth, fmt.Sprintf("%s %s", bearer, accessToken))
	req.Header.Set(headerKeyXeroTenantID, tenantID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return &ReusableRequest{
		Request: req,
	}, nil
}

func (c *client) EmployeeLeaveApplication(ctx context.Context, req *ReusableRequest) error {
	var d time.Duration

	retryCtx, cancel, backOff := newRetry(ctx, c.RateLimitBackoff, c.RateLimitTimeout)
	defer cancel()

	for {
		err := c.employeeLeaveApplication(ctx, req.Request)
		if err != nil {
			if errors.Is(err, unauthorized) {
				return err
			}

			if errors.Is(err, exceededRateLimit) {
				d = backOff.Pause()
			}

			if !errors.Is(err, nonRetryable) {
				if innerErr := gax.Sleep(retryCtx, d); innerErr != nil {
					return fmt.Errorf("failed, retry limit expired: %v", err)
				}
				continue
			}
			return err
		}

		return nil
	}
}

func (c *client) employeeLeaveApplication(ctx context.Context, req *http.Request) error {
	res, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute %s request. Cause %v, %w", empLeaveApplicationApiName, err, nonRetryable)
	}

	err = getHTTPStatusCode(ctx, res, empLeaveApplicationApiName)
	if err != nil {
		return err
	}

	return nil
}
