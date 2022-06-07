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

const apiName = "GetPayrollCalendars"

func (c *client) NewPayrollRequest(ctx context.Context, tenantID string) (*ReusableRequest, error) {
	contextLogger := log.WithContext(ctx)
	contextLogger.Info("Building new payroll request for tenant: ", tenantID)

	req, err := http.NewRequest(http.MethodGet, BuildXeroPayrollCalendarEndpoint(c.URL), nil)
	if err != nil {
		contextLogger.WithError(err).Errorf("failed to build HTTP request")
		return nil, err
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		contextLogger.WithError(err).Errorf("Error fetching the access token")
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("xero-tenant-id", tenantID)
	return &ReusableRequest{
		Request: req,
	}, nil
}

func (c *client) GetPayrollCalendars(ctx context.Context, req *ReusableRequest) (*PayrollCalendarResponse, error) {
	var d time.Duration

	retryCtx, cancel, backOff := newRetry(ctx)
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
					return nil, errors.New(fmt.Sprint("failed, retry limit expired:", err))
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
		return nil, errors.New(fmt.Sprint(fmt.Sprintf("failed to execute %s request ", apiName), err))
	}

	err = getHTTPStatusCode(ctx, res, apiName)
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

	response := &PayrollCalendarResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		contextLogger.WithError(err).Errorf("there was an error un marshalling the xero API resp. %v", err)
		return nil, err
	}

	return response, nil
}

func newRetry(ctx context.Context) (context.Context, context.CancelFunc, *gax.Backoff) {
	bo := NewDefaultBackoff()

	cctx, cancel := context.WithTimeout(ctx, bo.timeout)
	return cctx, cancel, bo.Backoff
}
