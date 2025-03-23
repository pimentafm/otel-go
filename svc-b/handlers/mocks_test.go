package handlers

import (
	"fmt"
	"svc-b/models"
)

type MockCEPService struct{}
type MockWeatherService struct{}

func (m *MockCEPService) GetCityByCEP(cep string) (string, error) {
	switch cep {
	case "22450000":
		return "Rio de Janeiro", nil
	case "123":
		return "", fmt.Errorf("invalid zipcode")
	case "99999999":
		return "", fmt.Errorf("can not find zipcode")
	default:
		return "", fmt.Errorf("unexpected error")
	}
}

func (m *MockWeatherService) GetTemperature(city string) (*models.Temperature, error) {
	if city == "Rio de Janeiro" {
		return &models.Temperature{
			TempC: 25.0,
			TempF: 77.0,
			TempK: 298.15,
		}, nil
	}
	return nil, fmt.Errorf("city not found")
}
