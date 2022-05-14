package xero

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/googleapis/gax-go/v2"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var (
	file = createTempFile("xero_session.json", []byte("{\n \"access_token\": \"e\",\n \"refresh_token\": \"cf6b89ee04bc5fa394c7b87f15439e3b3102e6fbd882ad5a0042a17ef99ba6b3\"\n}"))

	defaultClient = &client{
		URL:               "https://api.xero.com",
		AuthTokenLocation: file.Name(),
		RateLimitBackoff:  defaultRateLimitBackoff,
	}
)

func TestClient_GetPayrollCalendars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		client  *client
		want    *PayrollCalendarResponse
		handler func(w http.ResponseWriter, r *http.Request)
		err     error
	}{
		{
			name:   "200-success",
			client: defaultClient,
			want: &PayrollCalendarResponse{
				PayrollCalendars: []PayrollCalendar{
					{
						PayrollCalendarID: "4567891011",
						PaymentDate:       "/Date(632102400000+0000)/",
					},
				},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/payroll.xro/1.0/PayrollCalendars", r.RequestURI)
				_, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				res := PayrollCalendarResponse{
					PayrollCalendars: []PayrollCalendar{
						{
							PayrollCalendarID: "4567891011",
							PaymentDate:       "/Date(632102400000+0000)/",
						},
					},
				}
				c, err := json.Marshal(res)
				require.NoError(t, err)

				w.Write(c)
			},
		},
		{
			name:   "401-Unauthorized",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			err: errors.New("failed to call GetPayrollCalendars with cause 401 unauthorized"),
		},
		{
			name:   "403-Forbidden",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			err: errors.New("failed to call GetPayrollCalendars with cause 403 unauthorized"),
		},
		{
			name:   "400-BadRequest",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			err: errors.New("failed to call GetPayrollCalendars with cause 400 non retryable"),
		},
		{
			name:   "503-Unavailable",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			err: errors.New("failed to call GetPayrollCalendars with cause 503 non retryable"),
		},
		{
			name:   "429-RateLimit",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			err: errors.New("failed, retry limit expired:failed to call GetPayrollCalendars with cause 429 rate limit exceeded"),
		},
	}

	for _, test := range tests {
		tt := test
		ctx := context.Background()

		s := httptest.NewServer(http.HandlerFunc(tt.handler))
		tt.client.Client = s.Client()
		tt.client.URL = s.URL
		tt.client.RateLimitBackoff = &gax.Backoff{
			Initial:    time.Second,
			Max:        time.Second,
			Multiplier: 1,
		}

		gotReq, err := tt.client.NewPayrollRequest(ctx, "123")
		require.NoError(t, err)

		got, err := tt.client.GetPayrollCalendars(ctx, gotReq)
		if err != nil || tt.err != nil {
			require.EqualError(t, err, tt.err.Error())
		} else {
			require.Equal(t, tt.want, got)
		}
	}
}

func createTempFile(fileName string, content []byte) (f *os.File) {
	file, _ := ioutil.TempFile("", fileName)
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	_, err := file.Write(content)
	if err != nil {
		log.Fatalf("error writing temp file: %v", err)
		return nil
	}

	return file
}
