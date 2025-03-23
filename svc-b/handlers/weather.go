package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"svc-b/services"
	"time"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type WeatherHandler struct {
	cepService     services.CEPService
	weatherService services.WeatherService
	tracer         trace.Tracer
}

type CepRequest struct {
	Cep string `json:"cep"`
}

type WeatherResponse struct {
	City  string  `json:"city"`
	TempC float64 `json:"temp_C"`
	TempF float64 `json:"temp_F"`
	TempK float64 `json:"temp_K"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewWeatherHandler(cep services.CEPService, weather services.WeatherService) *WeatherHandler {
	return &WeatherHandler{
		cepService:     cep,
		weatherService: weather,
		tracer:         otel.Tracer("weather-handler"),
	}
}

func (h *WeatherHandler) GetWeatherByCEP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	ctx, span := h.tracer.Start(ctx, "GetWeatherByCEP")
	defer span.End()

	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	cep := vars["cep"]

	// Normalize CEP by removing non-numeric characters
	cep = strings.ReplaceAll(cep, "-", "")
	cep = strings.ReplaceAll(cep, ".", "")

	log.Printf("Recebida requisição para CEP: %s", cep)
	span.SetAttributes(attribute.String("cep", cep))

	h.processWeatherRequest(ctx, w, cep)
}

func (h *WeatherHandler) GetWeatherByCEPPost(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	ctx, span := h.tracer.Start(ctx, "GetWeatherByCEPPost")
	defer span.End()

	w.Header().Set("Content-Type", "application/json")

	var req CepRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request format")
		return
	}

	// Normalize CEP by removing non-numeric characters
	req.Cep = strings.ReplaceAll(req.Cep, "-", "")
	req.Cep = strings.ReplaceAll(req.Cep, ".", "")

	log.Printf("Recebida requisição POST para CEP: %s", req.Cep)
	span.SetAttributes(attribute.String("cep", req.Cep))

	h.processWeatherRequest(ctx, w, req.Cep)
}

func (h *WeatherHandler) processWeatherRequest(ctx context.Context, w http.ResponseWriter, cep string) {
	ctx, span := h.tracer.Start(ctx, "processWeatherRequest")
	defer span.End()

	if len(cep) != 8 {
		h.respondWithError(w, http.StatusUnprocessableEntity, "invalid zipcode")
		return
	}

	// Get city by CEP
	city, err := h.cepService.GetCityByCEP(ctx, cep)
	if err != nil {
		h.handleCEPError(w, err)
		return
	}

	// Get temperature for city
	temp, err := h.weatherService.GetTemperature(ctx, city)
	if err != nil {
		h.handleWeatherError(w, err)
		return
	}

	// Return successful response
	response := WeatherResponse{
		City:  city,
		TempC: temp.TempC,
		TempF: temp.TempF,
		TempK: temp.TempK,
	}

	h.respondWithJSON(w, http.StatusOK, response)
}

func (h *WeatherHandler) handleCEPError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrInvalidZipCode):
		h.respondWithError(w, http.StatusUnprocessableEntity, "invalid zipcode")
	case errors.Is(err, services.ErrZipCodeNotFound):
		h.respondWithError(w, http.StatusNotFound, "can not find zipcode")
	default:
		log.Printf("CEP Service error: %v", err)
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
	}
}

func (h *WeatherHandler) handleWeatherError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrAPIKeyNotConfigured):
		h.respondWithError(w, http.StatusInternalServerError, "weather service configuration error")
	case errors.Is(err, services.ErrCityNotFound):
		h.respondWithError(w, http.StatusNotFound, "city not found in weather service")
	default:
		log.Printf("Weather Service error: %v", err)
		h.respondWithError(w, http.StatusInternalServerError, "failed to get weather data")
	}
}

func (h *WeatherHandler) respondWithError(w http.ResponseWriter, code int, message string) {
	h.respondWithJSON(w, code, ErrorResponse{Error: message})
}

func (h *WeatherHandler) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling JSON response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
		return
	}

	w.WriteHeader(code)
	w.Write(response)
}
