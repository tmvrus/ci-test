package repositories

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
)

var (
	ErrMetricNotFound        = errors.New("metric not found")
	ErrCantParseCounter      = errors.New("can't parse counter delta")
	ErrCantParseGauge        = errors.New("can't parse gauge value")
	ErrUndefinedMetricType   = errors.New("metric type is not defined")
	ErrIncorrectHash         = errors.New("hash is not correct")
	ErrUnableUpdateCounter   = errors.New("unable update counter")
	ErrUnableUpdateGauge     = errors.New("unable update gauge")
	ErrIncorrectCounterValue = errors.New("incorrect counter value")
	ErrIncorrectGaugeValue   = errors.New("incorrect gauge value")
)

type Metrics struct {
	ID    string   `json:"id"`              // имя метрики
	MType string   `json:"type"`            // параметр, принимающий значение gauge или counter
	Delta *int64   `json:"delta,omitempty"` // значение метрики в случае передачи counter
	Value *float64 `json:"value,omitempty"` // значение метрики в случае передачи gauge
	Hash  string   `json:"hash,omitempty"`  // значение хеш-функции
}

func (m *Metrics) FromJSON(input io.Reader) error {
	if err := json.NewDecoder(input).Decode(m); err != nil {
		return err
	}
	return nil
}

func (m *Metrics) ToJSON(input io.Reader) error {
	b, err := io.ReadAll(input)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	return nil
}

func (m Metrics) String() string {
	delta := int64(0)
	value := float64(0)

	if m.Delta != nil {
		delta = *m.Delta
	}

	if m.Value != nil {
		value = *m.Value
	}

	return fmt.Sprintf("ID: %v, Type: %v, Delta: %v, Value: %v", m.ID, m.MType, delta, value)
}

type Storage interface {
	UpdateBatchMetrics([]Metrics) error
	UpdateGaugeMetrics(name, value string) error
	UpdateCounterMetrics(name, value string) (int64, error)
	GetGaugeMetrics(name string) (string, error)
	GetCounterMetrics(name string) (string, error)
	GetAllGaugeMetrics() []Metrics
	GetAllCounterMetrics() []Metrics
	Ping() error
}

type FileRepository interface {
	Sync() bool
	LoadFromDisk() ([]Metrics, error)
	SaveAllToDisk(m []Metrics) error
	SaveToDisk(m Metrics) error
	FileWriterClose() error
	FileReaderClose() error
}

type Storager struct {
	Repo     Storage
	FileRepo FileRepository
	Key      string
}

func NewStorager(storage Storage, fileRepo FileRepository, key string) Storager {
	return Storager{
		Repo:     storage,
		FileRepo: fileRepo,
		Key:      key,
	}
}

func (s *Storager) GetMetric(m Metrics) (Metrics, error) {
	switch m.MType {
	case "counter":
		v, err := s.Repo.GetCounterMetrics(m.ID)
		if err != nil {
			log.Printf("Error: %v \n", err)
			return m, ErrMetricNotFound
		}
		x, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			log.Printf("Error: %v", err)
			return m, ErrCantParseCounter
		}
		m.Delta = &x
		if s.Key != "" {
			m.Hash = hash(fmt.Sprintf("%s:counter:%d", m.ID, *m.Delta), s.Key)
		}
	case "gauge":
		v, err := s.Repo.GetGaugeMetrics(m.ID)
		if err != nil {
			log.Printf("Error: %v", err)
			return m, ErrMetricNotFound
		}
		x, err := strconv.ParseFloat(v, 64)
		if err != nil {
			log.Printf("Error: %v", err)
			return m, ErrCantParseGauge
		}
		m.Value = &x
		if s.Key != "" {
			m.Hash = hash(fmt.Sprintf("%s:gauge:%f", m.ID, *m.Value), s.Key)
		}
	default:
		return m, ErrUndefinedMetricType
	}

	return m, nil
}

func (s *Storager) UpdateBatchMetrics(metrics []Metrics) error {
	err := s.Repo.UpdateBatchMetrics(metrics)
	if err != nil {
		return err
	}
	return nil
}

func (s *Storager) UpdateMetrics(metrics Metrics) (Metrics, error) {
	switch metrics.MType {
	case "counter":
		if metrics.Delta != nil {
			if s.Key != "" {
				hash := hash(fmt.Sprintf("%s:counter:%d", metrics.ID, *metrics.Delta), s.Key)
				if hash != metrics.Hash {
					log.Println(ErrIncorrectHash)
					return Metrics{}, ErrIncorrectHash
				}
			}
			v, err := s.Repo.UpdateCounterMetrics(metrics.ID, fmt.Sprintf("%v", *metrics.Delta))
			if err != nil {
				log.Printf("Error: %v", err)
				return Metrics{}, ErrUnableUpdateCounter
			}
			metrics.Delta = &v
		} else {
			log.Printf("Error: Delta for %v Not Provided", metrics.ID)
			return Metrics{}, ErrIncorrectCounterValue
		}

	case "gauge":
		if metrics.Value != nil {
			if s.Key != "" {
				hash := hash(fmt.Sprintf("%s:gauge:%f", metrics.ID, *metrics.Value), s.Key)
				if hash != metrics.Hash {
					log.Println("Hash is not correct")
					return Metrics{}, ErrIncorrectHash
				}
			}
			err := s.Repo.UpdateGaugeMetrics(metrics.ID, fmt.Sprintf("%v", *metrics.Value))
			if err != nil {
				log.Printf("Error: %v", err)
				return Metrics{}, ErrUnableUpdateGauge
			}
		} else {
			log.Printf("Error: Value for %v Not Provided", metrics.ID)
			return Metrics{}, ErrIncorrectGaugeValue
		}
	default:
		return Metrics{}, ErrUndefinedMetricType
	}

	if s.FileRepo != nil && s.FileRepo.Sync() {
		if err := s.FileRepo.SaveToDisk(metrics); err != nil {
			log.Printf("Unable to sync Metric to file. \n Metric: %v", metrics)
		}
	}
	return metrics, nil
}

func (s *Storager) GetAllMetrics() map[string]string {
	counterData := s.Repo.GetAllCounterMetrics()
	gaugeData := s.Repo.GetAllGaugeMetrics()

	data := make(map[string]string)

	for _, v := range counterData {
		data[v.ID] = fmt.Sprintf("%v", *v.Delta)
	}

	for _, v := range gaugeData {
		data[v.ID] = fmt.Sprintf("%v", *v.Value)
	}
	return data
}

func hash(s, k string) string {
	data := []byte(s)
	key := []byte(k)

	h := hmac.New(sha256.New, key)
	h.Write(data)
	sign := h.Sum(nil)
	return hex.EncodeToString(sign)
}
