package xero

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_GetEmployees(t *testing.T) {

	tests := []struct {
		name    string
		client  *client
		want    *EmpResponse
		handler func(w http.ResponseWriter, r *http.Request)
		err     error
	}{
		{
			name:   "200-success",
			client: defaultClient,
			want: &EmpResponse{
				Employees: []Employee{
					{
						EmployeeID:        "123456",
						FirstName:         "Syril",
						LastName:          "Sadasivan",
						Status:            "Active",
						PayrollCalendarID: "4567891011",
					},
				},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/payroll.xro/1.0/Employees?page=1", r.RequestURI)
				_, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				res := EmpResponse{
					Employees: []Employee{
						{
							EmployeeID:        "123456",
							FirstName:         "Syril",
							LastName:          "Sadasivan",
							Status:            "Active",
							PayrollCalendarID: "4567891011",
						},
					},
				}
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
				require.Equal(t, "/payroll.xro/1.0/Employees?page=1", r.RequestURI)
				_, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				res := "™™¡¡¡¡ß"
				c, err := json.Marshal(res)
				require.NoError(t, err)

				_, err = w.Write(c)
				require.NoError(t, err)
			},
			err: errors.New("there was an error un marshalling the GetEmployees resp. cause: json: cannot unmarshal string into Go value of type xero.EmpResponse non retryable"),
		},
		{
			name:   "401-Unauthorized",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			err: errors.New("failed to call GetEmployees with cause 401 unauthorized"),
		},
		{
			name:   "403-Forbidden",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			err: errors.New("failed to call GetEmployees with cause 403 unauthorized"),
		},
		{
			name:   "400-BadRequest",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			err: errors.New("failed to call GetEmployees with cause 400 non retryable"),
		},
		{
			name:   "503-Unavailable",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			err: errors.New("failed to call GetEmployees with cause 503 non retryable"),
		},
		{
			name:   "429-RateLimit",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			err: errors.New("failed, retry limit expired: failed to call GetEmployees with cause 429 rate limit exceeded"),
		},
	}

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
			require.EqualError(t, err, tt.err.Error())
		} else {
			require.Equal(t, tt.want, got)
		}
	}
}

func Test_EmployeeLeaveBalance(t *testing.T) {

	tests := []struct {
		name    string
		client  *client
		want    *LeaveBalanceResponse
		handler func(w http.ResponseWriter, r *http.Request)
		err     error
	}{
		{
			name:   "200-success",
			client: defaultClient,
			want: &LeaveBalanceResponse{
				Employees: []Employee{
					{
						EmployeeID:        "123456",
						FirstName:         "Syril",
						LastName:          "Sadasivan",
						Status:            "Active",
						PayrollCalendarID: "4567891011",
					},
				},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/payroll.xro/1.0/Employees/1", r.RequestURI)
				_, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				res := EmpResponse{
					Employees: []Employee{
						{
							EmployeeID:        "123456",
							FirstName:         "Syril",
							LastName:          "Sadasivan",
							Status:            "Active",
							PayrollCalendarID: "4567891011",
						},
					},
				}
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
				require.Equal(t, "/payroll.xro/1.0/Employees/1", r.RequestURI)
				_, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				res := "™™¡¡¡¡ß"
				c, err := json.Marshal(res)
				require.NoError(t, err)

				_, err = w.Write(c)
				require.NoError(t, err)
			},
			err: errors.New("there was an error un marshalling the EmployeeLeaveBalance resp. cause: json: cannot unmarshal string into Go value of type xero.LeaveBalanceResponse non retryable"),
		},
		{
			name:   "401-Unauthorized",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			err: errors.New("failed to call GetEmployeeLeaveBalance with cause 401 unauthorized"),
		},
		{
			name:   "403-Forbidden",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			err: errors.New("failed to call GetEmployeeLeaveBalance with cause 403 unauthorized"),
		},
		{
			name:   "400-BadRequest",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			err: errors.New("failed to call GetEmployeeLeaveBalance with cause 400 non retryable"),
		},
		{
			name:   "503-Unavailable",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			err: errors.New("failed to call GetEmployeeLeaveBalance with cause 503 non retryable"),
		},
		{
			name:   "429-RateLimit",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			err: errors.New("failed, retry limit expired: failed to call GetEmployeeLeaveBalance with cause 429 rate limit exceeded"),
		},
	}

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
			require.EqualError(t, err, tt.err.Error())
		} else {
			require.Equal(t, tt.want, got)
		}
	}
}

func Test_EmployeeLeaveApplication(t *testing.T) {

	tests := []struct {
		name    string
		client  *client
		want    *LeaveBalanceResponse
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
			require.EqualError(t, err, tt.err.Error())
		} else {
			require.NoError(t, err)
		}
	}
}
