package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var (
	ErrInvalidZipCode  = errors.New("invalid zipcode")
	ErrZipCodeNotFound = errors.New("can not find zipcode")
	ErrInternalServer  = errors.New("internal server error")
)

type ViaCEPResponse struct {
	Cep         string `json:"cep"`
	Logradouro  string `json:"logradouro"`
	Complemento string `json:"complemento"`
	Bairro      string `json:"bairro"`
	Localidade  string `json:"localidade"`
	UF          string `json:"uf"`
	Erro        bool   `json:"erro"`
}

type ViaCEPService struct {
	client  HTTPClient
	baseURL string
}

func NewViaCEPService(client HTTPClient) *ViaCEPService {
	return &ViaCEPService{
		client:  client,
		baseURL: "https://viacep.com.br/ws/%s/json/",
	}
}

func (s *ViaCEPService) GetCityByCEP(ctx context.Context, cep string) (string, error) {
	tracer := otel.Tracer("viacep-service")
	ctx, span := tracer.Start(ctx, "ViaCEP-GetCityByCEP")
	defer span.End()

	// Normalize CEP by removing non-numeric characters
	cep = strings.ReplaceAll(cep, "-", "")
	cep = strings.ReplaceAll(cep, ".", "")

	log.Printf("Buscando CEP: %s", cep)
	span.SetAttributes(attribute.String("cep", cep))

	if len(cep) != 8 {
		span.SetStatus(codes.Error, "invalid zipcode format")
		return "", ErrInvalidZipCode
	}

	url := fmt.Sprintf(s.baseURL, cep)
	log.Printf("Fazendo requisição para: %s", url)
	span.SetAttributes(attribute.String("url", url))

	// Create a context with timeout if not already set
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("Erro ao criar requisição: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		log.Printf("Erro ao fazer requisição: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		log.Printf("Status code inválido: %d", resp.StatusCode)
		span.SetStatus(codes.Error, fmt.Sprintf("invalid status code: %d", resp.StatusCode))
		return "", ErrZipCodeNotFound
	}

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Erro ao ler corpo da resposta: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return "", ErrInternalServer
	}

	// Log response for debugging
	bodyString := string(bodyBytes)
	log.Printf("Resposta da API ViaCEP: %s", bodyString)

	// Parse response
	var viacepResponse ViaCEPResponse
	if err := json.Unmarshal(bodyBytes, &viacepResponse); err != nil {
		log.Printf("Erro ao decodificar resposta JSON: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return "", ErrInternalServer
	}

	// Check for errors reported by the API
	if viacepResponse.Erro {
		log.Printf("CEP não encontrado: resposta indica erro")
		span.SetStatus(codes.Error, "zipcode not found")
		return "", ErrZipCodeNotFound
	}

	// Validate city field
	if viacepResponse.Localidade == "" {
		log.Printf("CEP sem localidade")
		span.SetStatus(codes.Error, "empty city in response")
		return "", ErrZipCodeNotFound
	}

	log.Printf("Cidade encontrada: %s", viacepResponse.Localidade)
	span.SetAttributes(attribute.String("city", viacepResponse.Localidade))
	return viacepResponse.Localidade, nil
}
