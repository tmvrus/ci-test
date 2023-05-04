package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var listMetrics = []string{"Alloc", "BuckHashSys", "Frees", "RandomValue"}

func Test_collectMetrics(t *testing.T) {
	listMetrics := []string{"Alloc", "BuckHashSys", "Frees", "RandomValue"}

	mValue := &MetricValues{
		Gauge:     make(map[string]gauge),
		PollCount: 0,
	}

	r := gauge(rand.Float64())
	for _, v := range listMetrics {
		if v == "RandomValue" {
			mValue.Gauge[v] = r
			continue
		}
		mValue.Gauge[v] = 0
	}

	collectMetrics(mValue)

	for _, v := range listMetrics {
		if v == "RandomValue" {
			assert.NotEqual(t, r, v)
			assert.NotNil(t, v)
			continue
		}
		assert.NotNil(t, mValue.Gauge[v])

	}

	assert.NotNil(t, mValue.PollCount)

}

func Test_newMetricValues(t *testing.T) {

	tests := []struct {
		name string
		args []string
		want *MetricValues
	}{
		{
			name: "Testing with fields",
			args: listMetrics,
			want: &MetricValues{
				Gauge: map[string]gauge{
					"Alloc":       0,
					"BuckHashSys": 0,
					"Frees":       0,
					"RandomValue": 0,
				},
				PollCount: 0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newMetricValues(tt.args)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckGaugeMetrics(t *testing.T) {
	gaugeServer := float64(0.00)

	serv := Metrics{
		ID:    "HeapObjects",
		MType: "gauge",
		Value: &gaugeServer,
	}

	gaugeUpdate := float64(77.777)

	testsUpdate := Metrics{
		ID:    "HeapObjects",
		MType: "gauge",
		Value: &gaugeUpdate,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/value/", func(w http.ResponseWriter, r *http.Request) {
		v, err := json.Marshal(serv)
		require.NoError(t, err)
		w.Write(v)
	})

	mux.HandleFunc("/update/", func(w http.ResponseWriter, r *http.Request) {
		res, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		defer r.Body.Close()

		var v Metrics
		err = json.Unmarshal(res, &v)
		require.NoError(t, err)

		serv.Value = v.Value
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	j := Metrics{
		ID:    "HeapObjects",
		MType: "gauge",
	}

	client := http.Client{}
	api := API{
		Client:  &client,
		baseURL: ts.URL,
	}
	ctx := context.Background()

	initialRes, err := sendMetrics(j, ctx, api.baseURL, "/value/", api.Client)
	require.NoError(t, err)
	var initialVal Metrics
	err = json.Unmarshal(initialRes, &initialVal)
	require.NoError(t, err)

	_, err = sendMetrics(testsUpdate, ctx, api.baseURL, "/update/", api.Client)
	require.NoError(t, err)

	postRes, err := sendMetrics(j, ctx, api.baseURL, "/value/", api.Client)
	require.NoError(t, err)
	var postVal Metrics
	err = json.Unmarshal(postRes, &postVal)
	require.NoError(t, err)

	assert.NotEqual(t, *initialVal.Value, *postVal.Value)
	assert.Equal(t, *testsUpdate.Value, *postVal.Value)

}

func sendMetrics(m Metrics, ctx context.Context, baseURL, path string, client *http.Client) ([]byte, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unable send metric for URL: %s, \n Responce Status Code: %v", req.URL, res.StatusCode)
	}
	return body, nil

}
