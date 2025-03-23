package handlers

import (
	"github.com/gorilla/mux"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetWeatherByCEP(t *testing.T) {
	mockCEP := &MockCEPService{}
	mockWeather := &MockWeatherService{}
	handler := NewWeatherHandler(mockCEP, mockWeather)

	tests := []struct {
		name           string
		cep            string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Valid CEP",
			cep:            "22450000",
			expectedStatus: http.StatusOK,
			expectedBody:   `{"temp_C":25,"temp_F":77,"temp_K":298.15}`,
		},
		{
			name:           "Invalid CEP Format",
			cep:            "123",
			expectedStatus: http.StatusUnprocessableEntity,
			expectedBody:   `{"error":"invalid zipcode"}`,
		},
		{
			name:           "Non-existent CEP",
			cep:            "99999999",
			expectedStatus: http.StatusNotFound,
			expectedBody:   `{"error":"can not find zipcode"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/weather/"+tt.cep, nil)
			rr := httptest.NewRecorder()

			router := mux.NewRouter()
			router.HandleFunc("/weather/{cep}", handler.GetWeatherByCEP)
			router.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expectedStatus)
			}

			// Remova espaços em branco e nova linha para comparação
			gotBody := strings.TrimSpace(rr.Body.String())
			expectedBody := strings.TrimSpace(tt.expectedBody)
			if gotBody != expectedBody {
				t.Errorf("handler returned unexpected body: got %v want %v",
					gotBody, expectedBody)
			}
		})
	}
}
