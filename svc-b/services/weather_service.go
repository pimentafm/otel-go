package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"svc-b/models"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var (
	ErrAPIKeyNotConfigured = errors.New("weather API key not configured")
	ErrWeatherAPIFailed    = errors.New("weather API request failed")
	ErrCityNotFound        = errors.New("city not found")
)

type WeatherAPIService struct {
	client  HTTPClient
	baseURL string
}

type WeatherAPIResponse struct {
	Current struct {
		TempC float64 `json:"temp_c"`
		TempF float64 `json:"temp_f"`
	} `json:"current"`
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func NewWeatherAPIService(client HTTPClient) *WeatherAPIService {
	return &WeatherAPIService{
		client:  client,
		baseURL: "https://api.weatherapi.com/v1/current.json",
	}
}

func (s *WeatherAPIService) GetTemperature(ctx context.Context, city string) (*models.Temperature, error) {
	tracer := otel.Tracer("weather-api-service")
	ctx, span := tracer.Start(ctx, "WeatherAPI-GetTemperature")
	defer span.End()

	span.SetAttributes(attribute.String("city", city))

	apiKey := os.Getenv("WEATHER_API_KEY")
	if apiKey == "" {
		log.Printf("WEATHER_API_KEY não configurada")
		span.SetStatus(codes.Error, "API key not configured")
		return nil, ErrAPIKeyNotConfigured
	}

	encodedCity := url.QueryEscape(city)
	reqURL := fmt.Sprintf("%s?key=%s&q=%s", s.baseURL, apiKey, encodedCity)

	span.SetAttributes(attribute.String("url", s.baseURL))

	// Add timeout to context if not already set
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Implement retry logic
	var resp *http.Response
	var err error
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			log.Printf("Erro ao criar requisição para WeatherAPI (tentativa %d): %v", attempt, err)
			time.Sleep(time.Duration(attempt*100) * time.Millisecond)
			continue
		}

		resp, err = s.client.Do(req)
		if err == nil {
			break
		}

		log.Printf("Erro ao fazer requisição para WeatherAPI (tentativa %d): %v", attempt, err)
		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt*100) * time.Millisecond)
		}
	}

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("all weather API requests failed: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	var weatherResp WeatherAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&weatherResp); err != nil {
		log.Printf("Erro ao decodificar resposta da WeatherAPI: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to decode API response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Status code inválido da WeatherAPI: %d, error: %s",
			resp.StatusCode, weatherResp.Error.Message)
		span.SetStatus(codes.Error, weatherResp.Error.Message)

		// Check for city not found error (common error code: 1006)
		if weatherResp.Error.Code == 1006 {
			return nil, ErrCityNotFound
		}

		return nil, fmt.Errorf("%w: %s", ErrWeatherAPIFailed, weatherResp.Error.Message)
	}

	// Get and calculate temperatures
	tempC := weatherResp.Current.TempC

	// If TempF is provided by the API, use it directly
	var tempF float64
	if weatherResp.Current.TempF != 0 {
		tempF = weatherResp.Current.TempF
	} else {
		tempF = tempC*1.8 + 32
	}

	tempK := tempC + 273.15

	span.SetAttributes(
		attribute.Float64("temp_c", tempC),
		attribute.Float64("temp_f", tempF),
		attribute.Float64("temp_k", tempK),
	)

	return &models.Temperature{
		TempC: round(tempC, 2),
		TempF: round(tempF, 2),
		TempK: round(tempK, 2),
	}, nil
}

func round(num float64, places int) float64 {
	factor := float64(1)
	for i := 0; i < places; i++ {
		factor *= 10
	}
	return float64(int(num*factor+0.5)) / factor
}
