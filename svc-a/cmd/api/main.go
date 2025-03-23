package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

// Configuration holds all application configuration
type Config struct {
	Port        string
	ZipkinURL   string
	ServiceBURL string
	ServiceName string
	Timeout     time.Duration
}

// CepRequest represents the payload for a zipcode request
type CepRequest struct {
	Cep string `json:"cep"`
}

// WeatherResponse represents the weather data response
type WeatherResponse struct {
	City  string  `json:"city"`
	TempC float64 `json:"temp_C"`
	TempF float64 `json:"temp_F"`
	TempK float64 `json:"temp_K"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// LoadConfig loads configuration from environment variables with defaults
func LoadConfig() Config {
	return Config{
		Port:        getEnv("PORT", "8080"),
		ZipkinURL:   getEnv("ZIPKIN_URL", "http://zipkin:9411/api/v2/spans"),
		ServiceBURL: getEnv("SERVICE_B_URL", "http://svc-b:8081/weather"),
		ServiceName: getEnv("SERVICE_NAME", "svc-a"),
		Timeout:     time.Duration(getEnvAsInt("TIMEOUT_SECONDS", 10)) * time.Second,
	}
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt retrieves an environment variable as integer or returns a default value
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := getIntFromString(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getIntFromString converts a string to int with error handling
func getIntFromString(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

// initTracer initializes the OpenTelemetry tracer provider
func initTracer(config Config) (*sdktrace.TracerProvider, error) {
	exporter, err := zipkin.New(config.ZipkinURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create Zipkin exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(config.ServiceName),
			attribute.String("environment", getEnv("ENVIRONMENT", "production")),
		)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tracerProvider, nil
}

// App represents the application
type App struct {
	config Config
	tracer trace.Tracer
}

// NewApp creates a new application instance
func NewApp(config Config) *App {
	return &App{
		config: config,
		tracer: otel.Tracer(config.ServiceName),
	}
}

// respondWithError sends a JSON error response
func respondWithError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}

// HandleWeatherRequest handles the weather endpoint requests
func (app *App) HandleWeatherRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := app.tracer.Start(ctx, "HandleWeatherRequest")
	defer span.End()

	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "only POST method is allowed")
		span.SetAttributes(attribute.String("error", "method_not_allowed"))
		return
	}

	w.Header().Set("Content-Type", "application/json")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid request body")
		span.SetAttributes(attribute.String("error", "invalid_body"))
		return
	}

	var req CepRequest
	if err := json.Unmarshal(body, &req); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid request format")
		span.SetAttributes(attribute.String("error", "invalid_format"))
		return
	}

	cep := req.Cep
	span.SetAttributes(attribute.String("cep", cep))

	// Validate CEP
	if !isValidCEP(cep) {
		respondWithError(w, http.StatusUnprocessableEntity, "invalid zipcode")
		span.SetAttributes(attribute.String("error", "invalid_zipcode"))
		return
	}

	// Create a context with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, app.config.Timeout)
	defer cancel()

	// Call service B
	response, statusCode, err := app.callServiceB(ctxWithTimeout, cep)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("error calling service B: %v", err))
		span.SetAttributes(attribute.String("error", "service_b_error"))
		return
	}

	// Return service B's response
	w.WriteHeader(statusCode)
	w.Write(response)
}

// isValidCEP validates a Brazilian zipcode
func isValidCEP(cep string) bool {
	// Check if CEP is an 8-digit string
	if len(cep) != 8 {
		return false
	}

	// Check if it contains only digits
	for _, c := range cep {
		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}

// callServiceB calls the service B API
func (app *App) callServiceB(ctx context.Context, cep string) ([]byte, int, error) {
	ctx, span := app.tracer.Start(ctx, "CallServiceB")
	defer span.End()

	span.SetAttributes(attribute.String("cep", cep))

	reqData := CepRequest{Cep: cep}
	reqBody, err := json.Marshal(reqData)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, app.config.ServiceBURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Create HTTP client with timeouts and instrumentation
	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   app.config.Timeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response: %w", err)
	}

	span.SetAttributes(attribute.Int("status_code", resp.StatusCode))

	return respBody, resp.StatusCode, nil
}

// setupRoutes configures the HTTP routes
func (app *App) setupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Add otelhttp instrumentation to the handler
	handler := otelhttp.NewHandler(
		http.HandlerFunc(app.HandleWeatherRequest),
		"WeatherEndpoint",
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		}),
	)

	mux.Handle("/weather", handler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	return mux
}

func main() {
	// Configure structured logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting service...")

	// Load configuration
	config := LoadConfig()

	// Initialize the tracer
	tp, err := initTracer(config)
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}()

	// Create and configure the application
	app := NewApp(config)

	// Configure server
	server := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      app.setupRoutes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start the server
	log.Printf("Service-A starting on port %s", config.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start server: %v", err)
	}
}
