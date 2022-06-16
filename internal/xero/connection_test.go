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

func TestGetConnections(t *testing.T) {

	tests := []struct {
		name    string
		client  *client
		want    []Connection
		handler func(w http.ResponseWriter, r *http.Request)
		err     error
	}{
		{
			name:   "200-success",
			client: defaultClient,
			want: []Connection{
				{
					TenantID:   "123456",
					TenantType: "C",
					OrgName:    "DigIO",
				},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/connections", r.RequestURI)
				_, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				res := []Connection{
					{
						TenantID:   "123456",
						TenantType: "C",
						OrgName:    "DigIO",
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
				require.Equal(t, "/connections", r.RequestURI)
				_, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				res := "™™¡¡¡¡ß"

				c, err := json.Marshal(res)
				require.NoError(t, err)

				_, err = w.Write(c)
				require.NoError(t, err)
			},
			err: errors.New("there was an error un marshalling the xero API resp. json: cannot unmarshal string into Go value of type []xero.Connection"),
		},
		{
			name:   "Error-ReadingAuthToken",
			client: &client{},
			err:    errors.New("error fetching the access token. open : no such file or directory"),
		},
		{
			name:   "401-Unauthorized",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			err: errors.New("failed to call GetConnections with cause 401 unauthorized"),
		},
		{
			name:   "403-Forbidden",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			err: errors.New("failed to call GetConnections with cause 403 unauthorized"),
		},
		{
			name:   "400-BadRequest",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			err: errors.New("failed to call GetConnections with cause 400 non retryable"),
		},
		{
			name:   "503-Unavailable",
			client: defaultClient,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			err: errors.New("failed to call GetConnections with cause 503 non retryable"),
		},
	}

	for _, test := range tests {
		tt := test
		ctx := context.Background()

		s := httptest.NewServer(http.HandlerFunc(tt.handler))
		tt.client.Client = s.Client()
		tt.client.URL = s.URL

		got, err := tt.client.GetConnections(ctx)
		if err != nil || tt.err != nil {
			require.EqualError(t, err, tt.err.Error())
		} else {
			require.Equal(t, tt.want, got)
		}
	}
}
