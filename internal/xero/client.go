package xero

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"time"

	"github.com/googleapis/gax-go/v2"
	log "github.com/sirupsen/logrus"

	"github.com/syrilster/migrate-leave-krow-to-xero/internal/model"
)

const (
	payrollEndpoint             = "payroll.xro/1.0/PayrollCalendars"
	getEmployeesEndpoint        = "payroll.xro/1.0/Employees"
	empLeaveApplicationEndpoint = "payroll.xro/1.0/LeaveApplications"

	pageQueryParam = "page="
)

var (
	unauthorized      = errors.New("unauthorized")
	exceededRateLimit = errors.New("rate limit exceeded")
	nonRetryable      = errors.New("non retryable")

	defaultRateLimitBackoff = &gax.Backoff{
		Initial:    30 * time.Second,
		Max:        time.Minute,
		Multiplier: math.Phi,
	}
	defaultTimeout = 15 * time.Second
)

type ClientInterface interface {
	NewGetEmployeesRequest(ctx context.Context, tenantID string, page string) (*ReusableRequest, error)
	GetEmployees(ctx context.Context, req *ReusableRequest) (*EmpResponse, error)
	GetConnections(ctx context.Context) ([]Connection, error)
	NewEmployeeLeaveBalanceRequest(ctx context.Context, tenantID string, empID string) (*ReusableRequest, error)
	EmployeeLeaveBalance(ctx context.Context, req *ReusableRequest) (*LeaveBalanceResponse, error)
	NewEmployeeLeaveApplicationRequest(ctx context.Context, tenantID string, leaveReq LeaveApplicationRequest) (*ReusableRequest, error)
	EmployeeLeaveApplication(ctx context.Context, req *ReusableRequest) error
	GetPayrollCalendars(ctx context.Context, req *ReusableRequest) (*PayrollCalendarResponse, error)
	NewPayrollRequest(ctx context.Context, tenantID string) (*ReusableRequest, error)
}

type BackoffWithTimeout struct {
	Backoff *gax.Backoff
	Timeout time.Duration
}

// ReusableRequest can be used when a request containing body
// is static and user wants to reuse this request without rebuilding it again.
type ReusableRequest struct {
	*http.Request
	Body []byte
}

type RetryEndpoint struct {
	Name            string        `yaml:"name"`
	TotalLimit      time.Duration `yaml:"limit"`
	InitialEnvelope time.Duration `yaml:"initial"`
	MaxEnvelope     time.Duration `yaml:"max"`
	Multiplier      float64       `yaml:"multiplier"`
}

type client struct {
	*http.Client

	URL               string
	AuthTokenLocation string

	RateLimitBackoff *gax.Backoff
	RateLimitTimeout time.Duration
}

func NewDefaultBackoff() BackoffWithTimeout {
	return BackoffWithTimeout{
		Backoff: defaultRateLimitBackoff,
		Timeout: defaultTimeout,
	}
}

func New(endpoint string, tokenLoc string, timeout int) ClientInterface {
	return &client{
		Client:            http.DefaultClient,
		URL:               endpoint,
		AuthTokenLocation: tokenLoc,
		RateLimitBackoff:  defaultRateLimitBackoff,
		RateLimitTimeout:  time.Duration(timeout) * time.Minute,
	}
}

func (c *client) getAccessToken(ctx context.Context) (string, error) {
	var data *model.XeroResponse
	contextLogger := log.WithContext(ctx)
	sessionFile, err := ioutil.ReadFile(c.AuthTokenLocation)
	if err != nil {
		contextLogger.WithError(err).Errorf("error reading json file containing access token")
		return "", err
	}

	err = json.Unmarshal(sessionFile, &data)
	if err != nil {
		contextLogger.WithError(err).Errorf("error un marshalling json file containing access token")
		return "", err
	}
	return data.AccessToken, nil
}

func getHTTPStatusCode(ctx context.Context, res *http.Response, api string) error {
	contextLogger := log.WithContext(ctx)
	contextLogger.Infof("status returned from xero service %s ", res.Status)
	switch code := res.StatusCode; code {
	case http.StatusCreated, http.StatusOK:
		return nil

	case http.StatusTooManyRequests:
		return fmt.Errorf("failed to call %s with cause %d %w", api, code, exceededRateLimit)

	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("failed to call %s with cause %d %w", api, code, unauthorized)

	case http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed,
		http.StatusInternalServerError, http.StatusNotImplemented, http.StatusServiceUnavailable:
		return fmt.Errorf("failed to call %s with cause %d %w", api, code, nonRetryable)

	default:
		return fmt.Errorf("failed to call %s with cause %d", api, code)
	}
}

func newRetry(ctx context.Context, bo *gax.Backoff, timeout time.Duration) (context.Context, context.CancelFunc, *gax.Backoff) {
	b := BackoffWithTimeout{Backoff: bo, Timeout: timeout}

	cctx, cancel := context.WithTimeout(ctx, b.Timeout)
	return cctx, cancel, b.Backoff
}

func buildXeroPayrollCalendarEndpoint(url string) string {
	return fmt.Sprintf("%s/%s", url, payrollEndpoint)
}

func buildXeroEmployeesEndpoint(url, page string) string {
	return fmt.Sprintf("%s/%s?%s%s", url, getEmployeesEndpoint, pageQueryParam, page)
}

func buildXeroLeaveBalanceEndpoint(url, empID string) string {
	return fmt.Sprintf("%s/%s/%s", url, getEmployeesEndpoint, empID)
}

func buildXeroLeaveApplicationEndpoint(url string) string {
	return fmt.Sprintf("%s/%s", url, empLeaveApplicationEndpoint)
}
