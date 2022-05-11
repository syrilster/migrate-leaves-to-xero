package xero

import (
	"context"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
)

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

func (c *client) buildXeroConnectionsEndpoint() string {
	return c.URL + "/connections"
}
