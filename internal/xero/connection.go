package xero

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	log "github.com/sirupsen/logrus"
)

func (c *client) GetConnections(ctx context.Context) ([]Connection, error) {
	contextLogger := log.WithContext(ctx)

	httpRequest, err := http.NewRequest(http.MethodGet, c.buildXeroConnectionsEndpoint(), nil)
	if err != nil {
		return nil, err
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		msg := "error fetching the access token. %v"
		contextLogger.WithError(err).Errorf(msg, err)
		return nil, fmt.Errorf(msg, err)
	}

	httpRequest.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.Client.Do(httpRequest)
	if err != nil {
		contextLogger.WithError(err).Errorf("there was an error calling the xero connection API. %v", err)
		return nil, err
	}

	defer func() {
		if err = resp.Body.Close(); err != nil {
			fmt.Println("Error when closing:", err)
		}
	}()

	err = getHTTPStatusCode(ctx, resp, "GetConnections")
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		contextLogger.WithError(err).Errorf("error reading xero API data resp body (%s)", body)
		return nil, fmt.Errorf("error reading xero API data. Error: %v", err)
	}

	var response []Connection
	if err := json.Unmarshal(body, &response); err != nil {
		msg := "there was an error un marshalling the xero API resp. %v"
		contextLogger.WithError(err).Errorf(msg, err)
		return nil, fmt.Errorf(msg, err)
	}

	return response, nil
}

func (c *client) buildXeroConnectionsEndpoint() string {
	return c.URL + "/connections"
}
