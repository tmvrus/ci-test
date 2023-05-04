package dbstorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/fkocharli/metricity/internal/repositories"
)

var metricsList = []string{"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys", "HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased", "HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys", "MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC", "NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys", "Sys", "TotalAlloc", "RandomValue"}

type PostgreRepo struct {
	DB                  *sql.DB
	GaugeMetricsMutex   *sync.RWMutex
	CounterMetricsMutex *sync.RWMutex
}

func NewRepository(db *sql.DB) (*PostgreRepo, error) {

	p := &PostgreRepo{
		DB:                  db,
		GaugeMetricsMutex:   &sync.RWMutex{},
		CounterMetricsMutex: &sync.RWMutex{},
	}
	query := "CREATE TABLE IF NOT EXISTS metrics (metricID VARCHAR(50) UNIQUE NOT NULL, type varchar(20),counter bigint, gauge double precision)"
	ctx, cancelfunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelfunc()

	_, err := p.DB.ExecContext(ctx, query)
	if err != nil {
		log.Printf("Error %s when creating table", err)
		return nil, err
	}

	stmtGauge := "INSERT into metrics (metricID, type, gauge) values($1, $2, $3) ON CONFLICT DO NOTHING"

	for _, v := range metricsList {
		_, err := p.DB.Exec(stmtGauge, v, "gauge", float64(0.00))
		if err != nil {
			log.Printf("Error %s when inserting table", err)
			return nil, err
		}
	}

	stmtCounter := "INSERT into metrics (metricID, type, counter) values($1, $2, $3) ON CONFLICT DO NOTHING"

	_, err = p.DB.Exec(stmtCounter, "PollCount", "counter", int64(0))
	if err != nil {
		log.Printf("Error %s when inserting table", err)
		return nil, err
	}
	return p, nil
}

func (p *PostgreRepo) Ping() error {
	return p.DB.Ping()
}

func (p *PostgreRepo) UpdateGaugeMetrics(name, value string) error {
	g, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("unable to parse value to gauge. value: %v, error: %v", value, err)
	}

	p.GaugeMetricsMutex.Lock()
	defer p.GaugeMetricsMutex.Unlock()

	var stmtGauge string
	var count int
	err = p.DB.QueryRow("SELECT COUNT(*) FROM metrics where metricID = $1", name).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		stmtGauge = "UPDATE metrics SET gauge = $1 where metricID = $2"
	} else {
		stmtGauge = "INSERT into metrics (metricID, type, gauge) values($2, 'gauge', $1) ON CONFLICT DO NOTHING"

	}
	_, err = p.DB.Exec(stmtGauge, g, name)
	if err != nil {
		log.Printf("Error %s when inserting table", err)
		return err
	}

	return nil
}

func (p *PostgreRepo) UpdateCounterMetrics(name, value string) (int64, error) {
	g, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unable to parse value to counter. value: %v, error: %v", value, err)
	}

	p.CounterMetricsMutex.Lock()
	defer p.CounterMetricsMutex.Unlock()

	var stmtCounter string
	var count int
	err = p.DB.QueryRow("SELECT COUNT(*) FROM metrics where metricID = $1", name).Scan(&count)
	if err != nil {
		return 0, err
	}
	if count > 0 {

		var val int64
		query := "SELECT counter from metrics where metricID = $1"
		row := p.DB.QueryRow(query, name)

		err = row.Scan(&val)
		if err != nil {
			return 0, fmt.Errorf("unable to get stored counter value: %v, error: %v", val, err)
		}

		g += val
		stmtCounter = "UPDATE metrics SET counter = $1 where metricID = $2"

	} else {
		stmtCounter = "INSERT into metrics (metricID, type, counter) values($2, 'counter', $1) ON CONFLICT DO NOTHING"
	}

	_, err = p.DB.Exec(stmtCounter, g, name)
	if err != nil {
		log.Printf("Error %s when inserting table", err)
		return 0, err
	}

	return g, nil
}

func (p *PostgreRepo) UpdateBatchMetrics(metrics []repositories.Metrics) error {
	p.GaugeMetricsMutex.Lock()
	defer p.GaugeMetricsMutex.Unlock()
	p.CounterMetricsMutex.Lock()
	defer p.CounterMetricsMutex.Unlock()

	tx, err := p.DB.Begin()
	if err != nil {
		return err
	}

	gaugeStmt, err := tx.Prepare(`
	INSERT into metrics (metricID, type, gauge) 
	values($1, $2, $3) 
	ON CONFLICT (metricID) DO UPDATE
	SET gauge = $3 where metrics.metricID = $1
	`)
	if err != nil {
		log.Printf("Error on preparing transaction for Batch update gauge. Error: %v", err)
		return err
	}
	defer gaugeStmt.Close()

	counterStmt, err := tx.Prepare(`
	INSERT into metrics (metricID, type, counter) 
	values($1, $2, $3) 
	ON CONFLICT (metricID) DO UPDATE
	SET counter = COALESCE(metrics.counter,0) + $3 where metrics.metricID = $1
	`)
	if err != nil {
		log.Printf("Error on preparing transaction for Batch update counter. Error: %v", err)

		return err
	}
	defer counterStmt.Close()

	for _, v := range metrics {
		log.Printf("Updating metric:%v", v)
		switch v.MType {
		case "gauge":
			log.Printf("Updating Batch metric gauge: %v\n", v)

			if _, err = gaugeStmt.Exec(v.ID, v.MType, *v.Value); err != nil {
				log.Printf("Error on Batch update gauge. Error: %v", err)
				if err = tx.Rollback(); err != nil {
					log.Fatalf("update drivers: unable to rollback: %v", err)
				}
				return err
			}
		case "counter":
			log.Printf("Updating Batch metric counter: %v\n", v)

			if _, err = counterStmt.Exec(v.ID, v.MType, *v.Delta); err != nil {
				log.Printf("Error on Batch update counter. Error: %v", err)
				if err = tx.Rollback(); err != nil {
					log.Fatalf("update drivers: unable to rollback: %v", err)
				}
				return err
			}
		}
		log.Printf("Updated metric:%v", v)

	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("update drivers: unable to commit: %v", err)
		return err
	}

	return nil

}

func (p *PostgreRepo) GetGaugeMetrics(name string) (string, error) {
	p.GaugeMetricsMutex.RLock()
	defer p.GaugeMetricsMutex.RUnlock()

	var count int
	err := p.DB.QueryRow("SELECT COUNT(*) FROM metrics where metricID = $1", name).Scan(&count)
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "", errors.New("gauge value doesn't exist")
	}

	var val float64
	query := "SELECT gauge from metrics where metricID=$1"
	row := p.DB.QueryRow(query, name)

	err = row.Scan(&val)
	if err != nil {
		return "", fmt.Errorf("unable to get stored gauge value: %v. error: %v", val, err)
	}
	return fmt.Sprintf("%v", val), nil
}

func (p *PostgreRepo) GetCounterMetrics(name string) (string, error) {
	p.CounterMetricsMutex.RLock()
	defer p.CounterMetricsMutex.RUnlock()

	var count int
	err := p.DB.QueryRow("SELECT COUNT(*) FROM metrics where metricID = $1", name).Scan(&count)
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "", errors.New("count value doesn't exist")
	}
	var val int64
	query := "SELECT counter from metrics where metricID=$1"
	row := p.DB.QueryRow(query, name)

	err = row.Scan(&val)
	if err != nil {
		return "", fmt.Errorf("unable to  get stored counter value: %v. error: %v", val, err)
	}

	return fmt.Sprintf("%v", val), nil
}

func (p *PostgreRepo) GetAllGaugeMetrics() []repositories.Metrics {
	p.GaugeMetricsMutex.RLock()
	defer p.GaugeMetricsMutex.RUnlock()

	res := []repositories.Metrics{}

	query := "SELECT metricID, type, gauge FROM metrics WHERE type='gauge'"
	rows, err := p.DB.Query(query)
	if err != nil {
		log.Println(err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var r repositories.Metrics
		err = rows.Scan(&r.ID, &r.MType, &r.Value)
		if err != nil {
			log.Println(err)
		}

		res = append(res, r)
	}
	err = rows.Err()
	if err != nil {
		log.Println(err)
	}
	return res
}

func (p *PostgreRepo) GetAllCounterMetrics() []repositories.Metrics {
	p.CounterMetricsMutex.RLock()
	defer p.CounterMetricsMutex.RUnlock()

	res := []repositories.Metrics{}

	query := "SELECT metricID, type, counter FROM metrics WHERE type='counter'"
	rows, err := p.DB.Query(query)
	if err != nil {
		log.Println(err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var r repositories.Metrics
		err = rows.Scan(&r.ID, &r.MType, &r.Delta)
		if err != nil {
			log.Println(err)
		}

		res = append(res, r)
	}
	err = rows.Err()
	if err != nil {
		log.Println(err)
	}

	return res
}
