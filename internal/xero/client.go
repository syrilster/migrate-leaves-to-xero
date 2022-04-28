package xero

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/customhttp"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/model"
	"io/ioutil"
	"net/http"
	"strconv"
)

type ClientInterface interface {
	GetEmployees(ctx context.Context, tenantID string, page string) (*EmpResponse, error)
	GetConnections(ctx context.Context) ([]Connection, error)
	EmployeeLeaveBalance(ctx context.Context, tenantID string, empID string) (*LeaveBalanceResponse, error)
	EmployeeLeaveApplication(ctx context.Context, tenantID string, request LeaveApplicationRequest) error
	GetPayrollCalendars(ctx context.Context, tenantID string) (*PayrollCalendarResponse, error)
}

func NewClient(endpoint string, c customhttp.HTTPCommand, authTokenLoc string) *client {
	return &client{
		URL:               endpoint,
		HTTPCommand:       c,
		AuthTokenLocation: authTokenLoc,
	}
}

type client struct {
	URL               string
	HTTPCommand       customhttp.HTTPCommand
	AuthTokenLocation string
}

func (c *client) GetConnections(ctx context.Context) ([]Connection, error) {
	contextLogger := log.WithContext(ctx)

	httpRequest, err := http.NewRequest(http.MethodGet, c.buildXeroConnectionsEndpoint(), nil)
	if err != nil {
		return nil, err
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		contextLogger.WithError(err).Errorf("Error fetching the access token")
		return nil, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.HTTPCommand.Do(httpRequest)
	if err != nil {
		contextLogger.WithError(err).Errorf("there was an error calling the xero connection API. %v", err)
		return nil, err
	}

	defer func() {
		if err = resp.Body.Close(); err != nil {
			fmt.Println("Error when closing:", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		contextLogger.Infof("status returned from xero service %s ", resp.Status)
		return nil, fmt.Errorf("xero service (GetConnections) returned status: %s ", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		contextLogger.WithError(err).Errorf("error reading xero API data resp body (%s)", body)
		return nil, err
	}

	var response []Connection
	if err := json.Unmarshal(body, &response); err != nil {
		contextLogger.WithError(err).Errorf("there was an error un marshalling the xero API resp. %v", err)
		return nil, err
	}

	return response, nil
}

func (c *client) GetEmployees(ctx context.Context, tenantID string, page string) (*EmpResponse, error) {
	contextLogger := log.WithContext(ctx)
	contextLogger.Info("Fetching all employees for tenant: ", tenantID)
	httpRequest, err := http.NewRequest(http.MethodGet, c.buildXeroEmployeesEndpoint(page), nil)
	if err != nil {
		return nil, err
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		contextLogger.WithError(err).Errorf("Error fetching the access token")
		return nil, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+accessToken)
	httpRequest.Header.Set("xero-tenant-id", tenantID)

	contextLogger.Info("Calling Xero Employee Endpoint with page filter for page: ", page)
	resp, err := c.HTTPCommand.Do(httpRequest)
	if err != nil {
		contextLogger.WithError(err).Errorf("there was an error calling the xero connection API. %v", err)
		return nil, err
	}

	defer func() {
		if err = resp.Body.Close(); err != nil {
			fmt.Println("Error when closing:", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		contextLogger.Infof("status returned from xero service %s ", resp.Status)
		return nil, fmt.Errorf("xero service (GetEmployees) returned status: %s ", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		contextLogger.WithError(err).Errorf("error reading xero API data resp body (%s)", body)
		return nil, err
	}

	response := &EmpResponse{}
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

func (c *client) EmployeeLeaveBalance(ctx context.Context, tenantID string, empID string) (*LeaveBalanceResponse, error) {
	contextLogger := log.WithContext(ctx)
	contextLogger.Info("Fetching leave balance for employee: ", empID)
	httpRequest, err := http.NewRequest(http.MethodGet, c.buildXeroLeaveBalanceEndpoint(empID), nil)
	if err != nil {
		return nil, err
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		contextLogger.WithError(err).Errorf("Error fetching the access token")
		return nil, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+accessToken)
	httpRequest.Header.Set("xero-tenant-id", tenantID)

	resp, err := c.HTTPCommand.Do(httpRequest)
	if err != nil {
		contextLogger.WithError(err).Errorf("there was an error calling the xero connection API. %v", err)
		return nil, err
	}

	defer func() {
		if err = resp.Body.Close(); err != nil {
			fmt.Println("Error when closing:", err)
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
		contextLogger.WithError(err).Errorf("Error fetching the access token")
		return err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+accessToken)
	httpRequest.Header.Set("xero-tenant-id", tenantID)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	resp, err := c.HTTPCommand.Do(httpRequest)
	if err != nil {
		contextLogger.WithError(err).Errorf("there was an error calling the xero connection API. %v", err)
		return err
	}

	defer func() {
		if err = resp.Body.Close(); err != nil {
			fmt.Println("Error when closing:", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		contextLogger.Infof("status returned from xero service %s ", resp.Status)
		return fmt.Errorf("xero service (EmployeeLeaveApplication) returned status: %s ", resp.Status)
	}

	return nil
}

func (c *client) GetPayrollCalendars(ctx context.Context, tenantID string) (*PayrollCalendarResponse, error) {
	contextLogger := log.WithContext(ctx)
	contextLogger.Info("Fetching payoll calendar settings for tenant: ", tenantID)
	httpRequest, err := http.NewRequest(http.MethodGet, c.buildXeroPayrollCalendarEndpoint(), nil)
	if err != nil {
		return nil, err
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		contextLogger.WithError(err).Errorf("Error fetching the access token")
		return nil, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+accessToken)
	httpRequest.Header.Set("xero-tenant-id", tenantID)

	resp, err := c.HTTPCommand.Do(httpRequest)
	if err != nil {
		contextLogger.WithError(err).Errorf("there was an error calling the xero connection API. %v", err)
		return nil, err
	}

	defer func() {
		if err = resp.Body.Close(); err != nil {
			fmt.Println("Error when closing:", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		contextLogger.Infof("status returned from xero service %s ", resp.Status)
		return nil, fmt.Errorf("xero service (GetPayrollCalendars) returned status: %s ", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		contextLogger.WithError(err).Errorf("error reading xero API data resp body (%s)", body)
		return nil, err
	}

	response := &PayrollCalendarResponse{}
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

func (c *client) buildXeroConnectionsEndpoint() string {
	return c.URL + "/connections"
}

func (c *client) buildXeroEmployeesEndpoint(page string) string {
	return c.URL + "/payroll.xro/1.0/Employees?page=" + page
}

func (c *client) buildXeroLeaveBalanceEndpoint(empID string) string {
	return c.URL + "/payroll.xro/1.0/Employees/" + empID
}

func (c *client) buildXeroLeaveApplicationEndpoint() string {
	return c.URL + "/payroll.xro/1.0/LeaveApplications"
}

func (c *client) buildXeroPayrollCalendarEndpoint() string {
	return c.URL + "/payroll.xro/1.0/PayrollCalendars"
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
