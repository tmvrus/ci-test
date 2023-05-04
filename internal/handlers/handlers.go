package handlers

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/fkocharli/metricity/internal/repositories"
	"github.com/fkocharli/metricity/internal/server"

	"github.com/go-chi/chi/v5"
)

type ServerHandlers struct {
	*chi.Mux
	Storager repositories.Storager
}

func NewHandler(s repositories.Storager) *ServerHandlers {

	sh := &ServerHandlers{
		Mux:      server.NewRouter(),
		Storager: s,
	}

	sh.Mux.Post("/update/", sh.updateJSON)
	sh.Mux.Post("/updates/", sh.batchUpdates)
	sh.Mux.Post("/update/{type}/{metricname}/{metricvalue}", sh.update)

	sh.Mux.Post("/value/", sh.valueJSON)
	sh.Mux.Get("/value/{type}/{metricname}", sh.value)

	sh.Mux.Get("/ping", sh.ping)

	sh.Mux.Get("/", sh.home)
	return sh

}

func (s *ServerHandlers) batchUpdates(w http.ResponseWriter, r *http.Request) {
	metricsList := []repositories.Metrics{}

	var reader io.Reader

	if r.Header.Get(`Content-Encoding`) == `gzip` {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		reader = gz
		defer gz.Close()
	} else {
		reader = r.Body
	}

	if err := json.NewDecoder(reader).Decode(&metricsList); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()

	log.Printf("Received Batch Update for following metrics: %v", metricsList)

	err := s.Storager.UpdateBatchMetrics(metricsList)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *ServerHandlers) updateJSON(w http.ResponseWriter, r *http.Request) {

	var metrics repositories.Metrics

	if err := metrics.FromJSON(r.Body); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()

	log.Printf("Update Metric: %v\n", metrics)

	metrics, err := s.Storager.UpdateMetrics(metrics)
	if err != nil {
		switch err {
		case repositories.ErrIncorrectHash, repositories.ErrUndefinedMetricType, repositories.ErrIncorrectCounterValue, repositories.ErrIncorrectGaugeValue:
			w.WriteHeader(http.StatusBadRequest)
			return
		case repositories.ErrUnableUpdateCounter, repositories.ErrUnableUpdateGauge:
			w.WriteHeader(http.StatusInternalServerError)
			return
		default:
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
	}

	if err := json.NewEncoder(w).Encode(metrics); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

func (s *ServerHandlers) valueJSON(w http.ResponseWriter, r *http.Request) {
	var metrics repositories.Metrics

	if err := metrics.ToJSON(r.Body); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Printf("Get Metric: %v\n", metrics)

	metrics, err := s.Storager.GetMetric(metrics)
	if err != nil {
		switch err {
		case repositories.ErrMetricNotFound:
			w.WriteHeader(http.StatusNotFound)
			return
		case repositories.ErrCantParseCounter, repositories.ErrCantParseGauge:
			w.WriteHeader(http.StatusInternalServerError)
			return
		case repositories.ErrUndefinedMetricType:
			w.WriteHeader(http.StatusBadRequest)
			return
		default:
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
	}

	res, err := json.Marshal(metrics)
	if err != nil {
		log.Printf("Error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.Header().Add("Content-Type", "application/json")
	w.Write(res)
	w.WriteHeader(http.StatusOK)
}

func (s *ServerHandlers) update(w http.ResponseWriter, r *http.Request) {
	t := chi.URLParam(r, "type")
	n := chi.URLParam(r, "metricname")
	m := chi.URLParam(r, "metricvalue")

	if _, err := strconv.ParseFloat(m, 64); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	metrics := repositories.Metrics{ID: n, MType: t}

	if t == "counter" {
		n, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			log.Printf("Unable to parse counter: Error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		metrics.Delta = &n
	}

	if t == "gauge" {
		n, err := strconv.ParseFloat(m, 64)
		if err != nil {
			log.Printf("Unable to parse gauge: Error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		metrics.Value = &n
	}

	metrics, err := s.Storager.UpdateMetrics(metrics)
	if err != nil {
		switch err {
		case repositories.ErrIncorrectHash, repositories.ErrIncorrectCounterValue, repositories.ErrIncorrectGaugeValue:
			w.WriteHeader(http.StatusBadRequest)
			return
		case repositories.ErrUnableUpdateCounter, repositories.ErrUnableUpdateGauge:
			w.WriteHeader(http.StatusInternalServerError)
			return
		case repositories.ErrUndefinedMetricType:
			w.WriteHeader(http.StatusNotImplemented)
			return
		default:
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
	}

	w.Header().Add("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
}

func (s *ServerHandlers) value(w http.ResponseWriter, r *http.Request) {
	t := chi.URLParam(r, "type")
	n := chi.URLParam(r, "metricname")

	metrics := repositories.Metrics{ID: n, MType: t}

	metrics, err := s.Storager.GetMetric(metrics)
	if err != nil {
		switch err {
		case repositories.ErrMetricNotFound:
			w.WriteHeader(http.StatusNotFound)
			return
		case repositories.ErrCantParseCounter, repositories.ErrCantParseGauge:
			w.WriteHeader(http.StatusInternalServerError)
			return
		case repositories.ErrUndefinedMetricType:
			w.WriteHeader(http.StatusBadRequest)
			return
		default:
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
	}

	w.Header().Add("Content-Type", "text/plain")
	log.Println(metrics)
	w.WriteHeader(http.StatusOK)
	if metrics.MType == "gauge" {
		w.Write([]byte(fmt.Sprintf("%v", *metrics.Value)))
		return
	}
	w.Write([]byte(fmt.Sprintf("%v", *metrics.Delta)))
}

func (s *ServerHandlers) home(w http.ResponseWriter, r *http.Request) {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	tmplPath := filepath.Join(wd, "internal", "static", "html", "index.html")

	if filepath.Base(wd) == "server" {
		tmplPath = filepath.Join(filepath.Dir(filepath.Dir(wd)), "internal", "static", "html", "index.html")
	}

	t := template.Must(template.ParseFiles(tmplPath))
	data := s.Storager.GetAllMetrics()

	w.Header().Add("Content-Type", "text/html")
	t.Execute(w, data)
	w.WriteHeader(http.StatusOK)
}

func (s *ServerHandlers) ping(w http.ResponseWriter, r *http.Request) {
	err := s.Storager.Repo.Ping()
	if err != nil {
		log.Printf("Unable ping DB. Error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}
