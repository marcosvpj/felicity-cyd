// Package openmeteo is the infra layer: fetches hourly shortwave radiation
// forecasts from the Open-Meteo API.
package openmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

const defaultBaseURL = "https://api.open-meteo.com/v1/forecast"

// Response is the subset of the Open-Meteo /v1/forecast payload this client uses.
type Response struct {
	Hourly struct {
		Time               []string  `json:"time"`
		ShortwaveRadiation []float64 `json:"shortwave_radiation"`
	} `json:"hourly"`
}

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient() *Client {
	return &Client{httpClient: http.DefaultClient, baseURL: defaultBaseURL}
}

// NewClientWithBaseURL points the client at a different forecast endpoint,
// e.g. an httptest server or a deliberately unreachable host to exercise the
// "Open-Meteo is down" path in tests.
func NewClientWithBaseURL(baseURL string) *Client {
	return &Client{httpClient: http.DefaultClient, baseURL: baseURL}
}

// FetchShortwaveRadiation requests hourly shortwave_radiation for the given
// coordinates, aligned to America/Sao_Paulo so daily aggregation matches local days.
func (c *Client) FetchShortwaveRadiation(ctx context.Context, lat, lon float64, forecastDays int) (*Response, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}
	q := u.Query()
	q.Set("latitude", strconv.FormatFloat(lat, 'f', -1, 64))
	q.Set("longitude", strconv.FormatFloat(lon, 'f', -1, 64))
	q.Set("hourly", "shortwave_radiation")
	q.Set("timezone", "America/Sao_Paulo")
	q.Set("forecast_days", strconv.Itoa(forecastDays))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching forecast: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("closing open-meteo response body: %v", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo returned status %d", resp.StatusCode)
	}

	var out Response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding forecast response: %w", err)
	}
	return &out, nil
}
