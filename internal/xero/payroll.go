package xero

import (
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

const payrollApiName = "GetPayrollCalendars"

func (c *client) NewPayrollRequest(ctx context.Context, tenantID string) (*ReusableRequest, error) {
	contextLogger := log.WithContext(ctx)
	contextLogger.Info("Building new payroll request for tenant: ", tenantID)

	req, err := http.NewRequest(http.MethodGet, buildXeroPayrollCalendarEndpoint(c.URL), nil)
	if err != nil {
		contextLogger.WithError(err).Errorf("failed to build HTTP request")
		return nil, err
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		msg := "error fetching the access token. Cause %v"
		contextLogger.WithError(err).Errorf(msg, err)
		return nil, fmt.Errorf(msg, err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("xero-tenant-id", tenantID)
	return &ReusableRequest{
		Request: req,
	}, nil
}

func (c *client) GetPayrollCalendars(ctx context.Context, req *ReusableRequest) (*PayrollCalendarResponse, error) {
	var d time.Duration

	retryCtx, cancel, backOff := newRetry(ctx, c.RateLimitBackoff, c.RateLimitTimeout)
	defer cancel()

	for {
		res, err := c.getPayrollCalendars(ctx, req.Request)
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

func (c *client) getPayrollCalendars(ctx context.Context, req *http.Request) (*PayrollCalendarResponse, error) {
	contextLogger := log.WithContext(ctx)
	res, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute %s request. Cause %v, %w", payrollApiName, err, nonRetryable)
	}

	err = getHTTPStatusCode(ctx, res, payrollApiName)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		contextLogger.WithError(err).Errorf("error reading xero API data resp body (%s)", body)
		return nil, fmt.Errorf("error reading xero API data resp body. cause: %v %w", err, nonRetryable)
	}

	defer func() {
		if err = res.Body.Close(); err != nil {
			fmt.Println("Error when closing:", err)
		}
	}()

	response := &PayrollCalendarResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		contextLogger.WithError(err).Errorf("there was an error un marshalling the xero API resp. %v", err)
		return nil, fmt.Errorf("there was an error un marshalling the xero API resp. cause: %v %w", err, nonRetryable)
	}

	return response, nil
}
