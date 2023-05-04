package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fkocharli/metricity/internal/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockStorageType struct {
	MockField string
}

func (m MockStorageType) UpdateGaugeMetrics(name, value string) error {

	return nil
}

func (m MockStorageType) UpdateCounterMetrics(name, value string) (int64, error) {
	return 0, nil
}

func (m MockStorageType) GetGaugeMetrics(name string) (string, error) {
	return "", nil
}

func (m MockStorageType) GetCounterMetrics(name string) (string, error) {
	return "", nil
}

func (m MockStorageType) GetAllCounterMetrics() []repositories.Metrics {
	return nil
}

func (m MockStorageType) GetAllGaugeMetrics() []repositories.Metrics {
	return nil
}

func (m MockStorageType) UpdateBatchMetrics(metrics []repositories.Metrics) error {
	return nil
}
func (m MockStorageType) Ping() error {
	return nil
}

func TestHandlers(t *testing.T) {
	type want struct {
		contenType string
		statusCode int
	}

	type req struct {
		path    string
		method  string
		handler http.HandlerFunc
	}
	mockMemRepo := MockStorageType{}

	mockRepo := repositories.Storager{Repo: mockMemRepo, FileRepo: nil, Key: ""}

	handler := NewHandler(mockRepo)

	tests := []struct {
		name string
		req  req
		want want
	}{
		{
			name: "Update Gauge Metrics",
			req: req{
				path:    "/update/gauge/Sys/4.063232e+06",
				method:  http.MethodPost,
				handler: handler.update,
			},
			want: want{
				contenType: "text/plain",
				statusCode: http.StatusOK,
			},
		},
		{
			name: "Update Counter Metrics",
			req: req{
				path:    "/update/counter/Counter/10",
				method:  http.MethodPost,
				handler: handler.update,
			},
			want: want{
				contenType: "text/plain",
				statusCode: http.StatusOK,
			},
		},
	}

	r := NewHandler(mockRepo)
	s := httptest.NewServer(r)
	defer s.Close()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.req.method, s.URL+tt.req.path, nil)
			req.RequestURI = ""

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.want.contenType, resp.Header.Get("Content-Type"))
			assert.Equal(t, tt.want.statusCode, resp.StatusCode)
		})
	}
}
