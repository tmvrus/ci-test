package memorystorage

import (
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/fkocharli/metricity/internal/repositories"
)

type (
	gauge   float64
	counter int64
)

type GaugeMetrics map[string]gauge
type CounterMetrics map[string]counter

type MemStorage struct {
	GaugeMetrics        GaugeMetrics
	GaugeMetricsMutex   *sync.RWMutex
	CounterMetrics      CounterMetrics
	CounterMetricsMutex *sync.RWMutex
}

var metricsList = []string{"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys", "HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased", "HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys", "MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC", "NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys", "Sys", "TotalAlloc", "RandomValue"}

func NewRepository() *MemStorage {
	gaugeDefault := make(GaugeMetrics)
	for _, v := range metricsList {
		gaugeDefault[v] = gauge(0)
	}

	counterDefault := make(CounterMetrics)
	counterDefault["PollCount"] = 0

	return &MemStorage{
		GaugeMetrics:        gaugeDefault,
		GaugeMetricsMutex:   &sync.RWMutex{},
		CounterMetrics:      counterDefault,
		CounterMetricsMutex: &sync.RWMutex{},
	}
}

func (m *MemStorage) UpdateGaugeMetrics(name, value string) error {
	g, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("unable to get value to gauge. value: %v, error: %v", value, err)
	}

	m.GaugeMetricsMutex.Lock()
	defer m.GaugeMetricsMutex.Unlock()

	m.GaugeMetrics[name] = gauge(g)

	return nil
}

func (m *MemStorage) UpdateCounterMetrics(name, value string) (int64, error) {
	g, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("unable to parse value to counter. value: %v, error: %v", value, err)
	}

	m.CounterMetricsMutex.Lock()
	defer m.CounterMetricsMutex.Unlock()

	m.CounterMetrics[name] += counter(g)

	var v counter
	v, ok := m.CounterMetrics[name]
	if !ok {
		return 0, fmt.Errorf("unable to find stored counter value: %v", v)
	}

	return int64(v), nil
}
func (m *MemStorage) UpdateBatchMetrics(metrics []repositories.Metrics) error {
	for _, v := range metrics {
		switch v.MType {
		case "counter":
			m.CounterMetricsMutex.Lock()
			m.CounterMetrics[v.ID] += counter(*v.Delta)
			m.CounterMetricsMutex.Unlock()
		case "gauge":
			m.GaugeMetricsMutex.Lock()
			m.GaugeMetrics[v.ID] = gauge(*v.Value)
			m.GaugeMetricsMutex.Unlock()
		}

	}
	return nil
}

func (m *MemStorage) GetGaugeMetrics(name string) (string, error) {
	m.GaugeMetricsMutex.RLock()
	defer m.GaugeMetricsMutex.RUnlock()

	v, ok := m.GaugeMetrics[name]
	if !ok {
		return "", errors.New("Gauge metric not found. \n MetricID:" + name)
	}
	return fmt.Sprintf("%v", v), nil
}

func (m *MemStorage) GetCounterMetrics(name string) (string, error) {
	m.CounterMetricsMutex.RLock()
	defer m.CounterMetricsMutex.RUnlock()

	v, ok := m.CounterMetrics[name]
	if !ok {
		return "", errors.New("Counter metric not found. \n MetricID:" + name)
	}
	return fmt.Sprintf("%v", v), nil
}

func (m *MemStorage) GetAllGaugeMetrics() []repositories.Metrics {
	fmt.Println("GetAllGaugeMetrics before mutex")
	m.GaugeMetricsMutex.RLock()
	fmt.Println("GetAllGaugeMetrics after mutex")
	defer m.GaugeMetricsMutex.RUnlock()

	res := []repositories.Metrics{}

	for k, v := range m.GaugeMetrics {
		x := float64(v)
		res = append(res, repositories.Metrics{ID: k, MType: "gauge", Value: &x})
	}

	return res
}

func (m *MemStorage) GetAllCounterMetrics() []repositories.Metrics {
	m.CounterMetricsMutex.RLock()
	defer m.CounterMetricsMutex.RUnlock()

	res := []repositories.Metrics{}

	for k, v := range m.CounterMetrics {
		x := int64(v)
		res = append(res, repositories.Metrics{ID: k, MType: "counter", Delta: &x})
	}

	return res
}
func (m *MemStorage) Ping() error {
	return nil
}
