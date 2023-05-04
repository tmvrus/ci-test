package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"time"

	"github.com/fkocharli/metricity/internal/config"
)

type (
	gauge   float64
	counter int64
)

type MetricValues struct {
	Gauge     map[string]gauge
	PollCount counter
}

type API struct {
	Client  *http.Client
	baseURL string
}

type Metrics struct {
	ID    string   `json:"id"`              // имя метрики
	MType string   `json:"type"`            // параметр, принимающий значение gauge или counter
	Delta *int64   `json:"delta,omitempty"` // значение метрики в случае передачи counter
	Value *float64 `json:"value,omitempty"` // значение метрики в случае передачи gauge
	Hash  string   `json:"hash,omitempty"`  // значение хеш-функции
}

var (
	poolticket   *time.Ticker
	reportTicker *time.Ticker
	baseURL      string
)

func main() {
	cfg, err := config.NewConfig("agent")
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	poolticket = time.NewTicker(cfg.AgentConfig.PollInterval)
	reportTicker = time.NewTicker(cfg.AgentConfig.ReportInterval)
	baseURL = fmt.Sprintf("http://%s/", cfg.AgentConfig.Address)

	currentMetricsValue := newMetricValues(getMetricNames())

	RunAgent(currentMetricsValue, cfg.AgentConfig.Key)

}

func collectMetrics(metricList *MetricValues) {

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	v := reflect.ValueOf(&ms).Elem()

	metricList.PollCount++

	for k := range metricList.Gauge {
		if k == "RandomValue" {
			metricList.Gauge[k] = gauge(rand.Float64())
		}
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i).Name
			typ := v.Type().Field(i).Type.Kind()

			if field == k && typ == reflect.Uint64 {
				val := v.Field(i).Uint()
				metricList.Gauge[k] = gauge(val)
			}
		}
	}
}

func (a *API) makeRequest(m []Metrics, ctx context.Context, path string) (*http.Request, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		log.Printf("Unable Marshal request. Error: %v", err)
		return nil, err
	}

	p, err := compress(payload)
	if err != nil {
		log.Printf("Unable compress payload, error: %v", err)
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, bytes.NewBuffer(p))
	if err != nil {
		log.Printf("Unable send Batch for URL: %s \n Error: %s", a.baseURL+path, err)
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Content-Encoding", "gzip")

	return req, nil

}

func (a *API) sendMetricsRetryFallback(req, fallbackReq *http.Request) error {
	retries := 3

	for i := 0; i <= retries; i++ {
		res, err := a.Client.Do(req)
		if err != nil || res.StatusCode != http.StatusOK {
			log.Printf("unable send metric for url: %s, \n", req.URL)
			time.Sleep(time.Second)
			continue
		}
		io.ReadAll(res.Body)
		defer res.Body.Close()
		return nil
	}

	if fallbackReq != nil {
		log.Printf("Unable to send metrics to url: %v. Trying fallback url: %v", req.URL, fallbackReq.URL)
		fallbackRes, err := a.Client.Do(fallbackReq)
		if err != nil || fallbackRes.StatusCode != http.StatusOK {
			return fmt.Errorf("unable send metric for fallback url: %s", fallbackReq.URL)
		}
		io.ReadAll(fallbackRes.Body)
		defer fallbackRes.Body.Close()
	}

	return nil
}

func newMetricValues(metricList []string) *MetricValues {
	m := &MetricValues{
		Gauge:     make(map[string]gauge),
		PollCount: 0,
	}
	for _, v := range metricList {
		m.Gauge[v] = 0
	}
	return m
}

func RunAgent(metrics *MetricValues, key string) {

	client := http.Client{
		Timeout: 10 * time.Second,
	}
	ctx := context.Background()
	api := API{
		Client:  &client,
		baseURL: baseURL,
	}

	for {
		select {
		case <-poolticket.C:
			collectMetrics(metrics)
		case <-reportTicker.C:
			var metricsBucket []Metrics
			for k, v := range metrics.Gauge {

				val := float64(v)
				metricsJSON := Metrics{
					ID:    k,
					MType: "gauge",
					Value: &val,
				}
				if key != "" {
					metricsJSON.Hash = hash(fmt.Sprintf("%s:gauge:%f", metricsJSON.ID, *metricsJSON.Value), key)
				}

				metricsBucket = append(metricsBucket, metricsJSON)
			}
			del := int64(metrics.PollCount)
			metricsJSON := Metrics{
				ID:    "PollCount",
				MType: "counter",
				Delta: &del,
			}

			if key != "" {
				metricsJSON.Hash = hash(fmt.Sprintf("%s:counter:%d", metricsJSON.ID, *metricsJSON.Delta), key)
			}

			metricsBucket = append(metricsBucket, metricsJSON)

			req, err := api.makeRequest(metricsBucket, ctx, "updates/")
			if err != nil {
				log.Printf("Unable create request.\n Error: %s", err)
			}
			err = api.sendMetricsRetryFallback(req, nil)
			if err != nil {
				log.Printf("Unable send metric.\n Error: %s", err)
			}

		}
	}
}

func getMetricNames() []string {
	return []string{"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys", "HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased", "HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys", "MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC", "NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys", "Sys", "TotalAlloc", "RandomValue"}
}

func hash(s, k string) string {
	data := []byte(s)
	key := []byte(k)

	h := hmac.New(sha256.New, key)
	h.Write(data)
	sign := h.Sum(nil)

	return hex.EncodeToString(sign)
}

func compress(data []byte) ([]byte, error) {
	var b bytes.Buffer

	w := gzip.NewWriter(&b)

	_, err := w.Write(data)
	if err != nil {
		return nil, fmt.Errorf("failed write data to compress temporary buffer: %v", err)
	}

	err = w.Close()
	if err != nil {
		return nil, fmt.Errorf("failed compress data: %v", err)
	}

	return b.Bytes(), nil
}
