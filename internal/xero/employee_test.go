package xero

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetEmployees(t *testing.T) {

	tests := getTestCases[EmpResponse](t, &EmpResponse{
		Employees: []Employee{
			{
				EmployeeID:        "123456",
				FirstName:         "John",
				LastName:          "Coholan",
				Status:            "Active",
				PayrollCalendarID: "4567891011",
			},
		},
	}, "/payroll.xro/1.0/Employees?page=1", "GetEmployees")

	for _, test := range tests {
		tt := test
		ctx := context.Background()

		s := httptest.NewServer(http.HandlerFunc(tt.handler))
		tt.client.Client = s.Client()
		tt.client.URL = s.URL

		gotReq, err := tt.client.NewGetEmployeesRequest(ctx, "123", "1")
		require.NoError(t, err)

		got, err := tt.client.GetEmployees(ctx, gotReq)
		if err != nil || tt.err != nil {
			require.ErrorContains(t, err, tt.err.Error())
		} else {
			require.Equal(t, tt.want, got)
		}
	}
}

func TestEmployeeLeaveBalance(t *testing.T) {

	tests := getTestCases[LeaveBalanceResponse](t, &LeaveBalanceResponse{
		Employees: []Employee{
			{
				EmployeeID:        "123456",
				FirstName:         "John",
				LastName:          "Coholan",
				Status:            "Active",
				PayrollCalendarID: "4567891011",
			},
		},
	}, "/payroll.xro/1.0/Employees/1", "GetEmployeeLeaveBalance")

	for _, test := range tests {
		tt := test
		ctx := context.Background()

		s := httptest.NewServer(http.HandlerFunc(tt.handler))
		tt.client.Client = s.Client()
		tt.client.URL = s.URL

		gotReq, err := tt.client.NewEmployeeLeaveBalanceRequest(ctx, "123", "1")
		require.NoError(t, err)

		got, err := tt.client.EmployeeLeaveBalance(ctx, gotReq)
		if err != nil || tt.err != nil {
			require.ErrorContains(t, err, tt.err.Error())
		} else {
			require.Equal(t, tt.want, got)
		}
	}
}

func TestEmployeeLeaveApplication(t *testing.T) {
	tests := []struct {
		name    string
		client  *client
		handler func(w http.ResponseWriter, r *http.Request)
		err     error
	}{
		{
			name:   "200-success",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/payroll.xro/1.0/LeaveApplications", r.RequestURI)
				_, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				res := "dummy resp"
				c, err := json.Marshal(res)
				require.NoError(t, err)

				_, err = w.Write(c)
				require.NoError(t, err)
			},
		},
		{
			name:   "401-Unauthorized",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			err: errors.New("failed to call EmployeeLeaveApplication with cause 401 unauthorized"),
		},
		{
			name:   "403-Forbidden",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			err: errors.New("failed to call EmployeeLeaveApplication with cause 403 unauthorized"),
		},
		{
			name:   "400-BadRequest",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			err: errors.New("failed to call EmployeeLeaveApplication with cause 400 non retryable"),
		},
		{
			name:   "503-Unavailable",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			err: errors.New("failed to call EmployeeLeaveApplication with cause 503 non retryable"),
		},
		{
			name:   "429-RateLimit",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			err: errors.New("failed, retry limit expired: failed to call EmployeeLeaveApplication with cause 429 rate limit exceeded"),
		},
	}

	for _, test := range tests {
		tt := test
		ctx := context.Background()

		s := httptest.NewServer(http.HandlerFunc(tt.handler))
		tt.client.Client = s.Client()
		tt.client.URL = s.URL

		gotReq, err := tt.client.NewEmployeeLeaveApplicationRequest(ctx, "123", LeaveApplicationRequest{})
		require.NoError(t, err)

		err = tt.client.EmployeeLeaveApplication(ctx, gotReq)
		if err != nil || tt.err != nil {
			require.ErrorContains(t, err, tt.err.Error())
		} else {
			require.NoError(t, err)
		}
	}
}

func getTestCases[T interface{}](t *testing.T, mockRes *T, expectedInputURL string, apiName string) []struct {
	name    string
	client  *client
	want    *T
	handler func(w http.ResponseWriter, r *http.Request)
	err     error
} {
	tests := []struct {
		name    string
		client  *client
		want    *T
		handler func(w http.ResponseWriter, r *http.Request)
		err     error
	}{
		{
			name:   "200-success",
			client: defaultClient,
			want:   mockRes,
			handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, expectedInputURL, r.RequestURI)
				_, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				res := mockRes
				c, err := json.Marshal(res)
				require.NoError(t, err)

				_, err = w.Write(c)
				require.NoError(t, err)
			},
		},
		{
			name:   "Error-ReadingRespData",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, expectedInputURL, r.RequestURI)
				_, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				res := "™™¡¡¡¡ß"
				c, err := json.Marshal(res)
				require.NoError(t, err)

				_, err = w.Write(c)
				require.NoError(t, err)
			},
			err: fmt.Errorf("there was an error un marshalling the %s resp. cause: json: cannot unmarshal string into Go value", apiName),
		},
		{
			name:   "401-Unauthorized",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			err: fmt.Errorf("failed to call %s with cause 401 unauthorized", apiName),
		},
		{
			name:   "403-Forbidden",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			err: fmt.Errorf("failed to call %s with cause 403 unauthorized", apiName),
		},
		{
			name:   "400-BadRequest",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			err: fmt.Errorf("failed to call %s with cause 400 non retryable", apiName),
		},
		{
			name:   "503-Unavailable",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			err: fmt.Errorf("failed to call %s with cause 503 non retryable", apiName),
		},
		{
			name:   "429-RateLimit",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			err: fmt.Errorf("failed, retry limit expired: failed to call %s with cause 429 rate limit exceeded", apiName),
		},
	}

	return tests
}
