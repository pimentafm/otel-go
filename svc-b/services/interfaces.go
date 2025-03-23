package services

import (
	"context"
	"net/http"
	"svc-b/models"
)

// CEPService defines the interface for CEP lookup operations
type CEPService interface {
	GetCityByCEP(ctx context.Context, cep string) (string, error)
}

// WeatherService defines the interface for weather data operations
type WeatherService interface {
	GetTemperature(ctx context.Context, city string) (*models.Temperature, error)
}

// HTTPClient interface allows for mocking the HTTP client in tests
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}
