package filewriter

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fkocharli/metricity/internal/repositories"
)

type Filer struct {
	FileRepo      repositories.FileRepository
	Repo          repositories.Storage
	StoreInterval time.Duration
	Restore       bool
}

func New(fr repositories.FileRepository, r repositories.Storage, st time.Duration, res bool) *Filer {
	return &Filer{
		FileRepo:      fr,
		Repo:          r,
		StoreInterval: st,
		Restore:       res,
	}
}

func (f *Filer) Run(ctx context.Context) error {
	if f.Restore {
		log.Println("Restoring from file")
		f.Load()
	}
	var err error
	group := &sync.WaitGroup{}

	errChan := make(chan error, 1)

	if f.StoreInterval != 0 {
		group.Add(1)
		go func() {
			defer group.Done()
			f.SaveOnTick(ctx)
		}()
	}

	select {
	case <-ctx.Done():
		group.Add(1)
		log.Println("Starting Saving to disk")
		go func() {
			defer group.Done()
			err := f.Save()
			if err != nil {
				log.Println(err)
			}
		}()
	case err = <-errChan:
	}

	group.Wait()
	return err
}

func (f *Filer) Save() error {
	log.Println("Saving to disk")

	var metrics []repositories.Metrics

	metrics = append(metrics, f.Repo.GetAllCounterMetrics()...)
	metrics = append(metrics, f.Repo.GetAllGaugeMetrics()...)

	err := f.FileRepo.SaveAllToDisk(metrics)
	if err != nil {
		return err
	}
	return nil

}

func (f *Filer) SaveOnTick(ctx context.Context) {

	storageTimer := time.NewTicker(f.StoreInterval)
	defer storageTimer.Stop()

	var errChan chan error
	for {
		select {
		case <-storageTimer.C:
			err := f.Save()
			if err != nil {
				errChan <- err
			}
		case <-ctx.Done():
			return
		case err := <-errChan:
			log.Println(err)
		}
	}
}

func (f *Filer) Load() {

	var metrics []repositories.Metrics

	metrics, err := f.FileRepo.LoadFromDisk()
	if err != nil {
		log.Println(err)
	}

	for _, v := range metrics {
		switch v.MType {
		case "counter":
			_, err := f.Repo.UpdateCounterMetrics(v.ID, fmt.Sprintf("%v", *v.Delta))
			if err != nil {
				log.Printf("Unable to load counter metric: \n %v \n Error: %v", v, err)
			}
		case "gauge":
			err := f.Repo.UpdateGaugeMetrics(v.ID, fmt.Sprintf("%v", *v.Value))
			if err != nil {
				log.Printf("Unable to load gauge metric: \n %v \n Error: %v", v, err)
			}
		}
	}

	time.Sleep(10 * time.Second)

}
