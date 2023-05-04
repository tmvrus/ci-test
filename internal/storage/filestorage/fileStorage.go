package filestorage

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/fkocharli/metricity/internal/repositories"
)

type FileStore struct {
	FileReader    *os.File
	Decoder       *json.Decoder
	FileWriter    *os.File
	Encoder       *json.Encoder
	FileMutex     *sync.RWMutex
	StoreInterval time.Duration
}

func NewRepository(path string, s time.Duration) (*FileStore, error) {
	if path != "" {
		r, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0777)
		if err != nil {
			return nil, err
		}

		w, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0777)
		if err != nil {
			return nil, err
		}

		return &FileStore{
			FileReader:    r,
			Decoder:       json.NewDecoder(r),
			FileWriter:    w,
			Encoder:       json.NewEncoder(w),
			FileMutex:     &sync.RWMutex{},
			StoreInterval: s,
		}, nil

	}
	return nil, nil
}

func (f *FileStore) Sync() bool {
	return f.StoreInterval == 0
}

func (f *FileStore) SaveAllToDisk(m []repositories.Metrics) error {
	f.FileMutex.Lock()
	defer f.FileMutex.Unlock()

	f.FileWriter.Seek(0, 0)
	f.FileWriter.Truncate(0)

	f.Encoder.SetIndent("", "    ")
	if err := f.Encoder.Encode(m); err != nil {
		log.Printf("Unable to save to file Metrics. \n Metrics: %v \n Error: %v", m, err)
		return err
	}
	return nil
}

func (f *FileStore) SaveToDisk(m repositories.Metrics) error {
	f.FileMutex.Lock()
	defer f.FileMutex.Unlock()

	var x []repositories.Metrics

	f.FileReader.Seek(0, 0)

	for f.Decoder.More() {
		err := f.Decoder.Decode(&x)
		if err != io.EOF && err != nil {
			log.Printf("Unable to save to file Metric: %v \n Error: %v", m, err)
			return err
		}
	}

	metricExist := false
	if len(x) > 0 {
		for i, v := range x {
			if v.ID == m.ID {
				switch v.MType {
				case "counter":
					*x[i].Delta = *m.Delta
				case "gauge":
					*x[i].Value = *m.Value
				}
				metricExist = true
				break
			}
		}
	}
	if !metricExist {
		x = append(x, m)
	}

	f.FileWriter.Seek(0, 0)
	f.FileWriter.Truncate(0)

	f.Encoder.SetIndent("", "    ")

	if err := f.Encoder.Encode(x); err != nil {
		log.Printf("Unable to save to file Metric. \n Metric: %v \n Error: %v", m, err)
		return err
	}
	return nil

}

func (f *FileStore) LoadFromDisk() ([]repositories.Metrics, error) {
	f.FileMutex.RLock()
	defer f.FileMutex.RUnlock()

	f.FileReader.Seek(0, 0)

	var m []repositories.Metrics

	for f.Decoder.More() {
		err := f.Decoder.Decode(&m)
		if err != io.EOF && err != nil {
			log.Printf("Unable to read from file Metrics. \n Error: %v", err)
			return nil, err
		}
	}

	return m, nil
}

func (f *FileStore) FileWriterClose() error {
	return f.FileWriter.Close()
}

func (f *FileStore) FileReaderClose() error {
	return f.FileReader.Close()
}
