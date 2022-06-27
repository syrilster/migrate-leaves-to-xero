package xero

import (
	"context"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

var (
	file = createTempFile("xero_session.json", []byte("{\n \"access_token\": \"e\",\n \"refresh_token\": \"cf6b89ee04bc5fa394c7b87f15439e3b3102e6fbd882ad5a0042a17ef99ba6b3\"\n}"))

	defaultClient = &client{
		AuthTokenLocation: file.Name(),
		RateLimitBackoff:  defaultRateLimitBackoff,
		RateLimitTimeout:  defaultTimeout,
	}
)

func TestGetPayrollCalendars(t *testing.T) {
	t.Parallel()

	tests := getTestCases[PayrollCalendarResponse](t, &PayrollCalendarResponse{
		PayrollCalendars: []PayrollCalendar{
			{
				PayrollCalendarID: "4567891011",
				PaymentDate:       "/Date(632102400000+0000)/",
			},
		},
	}, "/payroll.xro/1.0/PayrollCalendars", "GetPayrollCalendars")

	for _, test := range tests {
		tt := test
		ctx := context.Background()

		s := httptest.NewServer(http.HandlerFunc(tt.handler))
		tt.client.Client = s.Client()
		tt.client.URL = s.URL

		gotReq, err := tt.client.NewPayrollRequest(ctx, "123")
		require.NoError(t, err)

		got, err := tt.client.GetPayrollCalendars(ctx, gotReq)
		if err != nil || tt.err != nil {
			require.ErrorContains(t, err, tt.err.Error())
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
