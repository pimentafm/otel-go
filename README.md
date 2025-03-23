
# Weather CEP

Weather CEP is a simple API that provides weather information based on the Brazilian postal code (CEP).

## Test
## Prerequisites

- Go 1.23.6+
- Docker
- Make
- Otel
- Zipkin

## Setup

1. Clone the repository:
    ```sh
    https://github.com/pimentafm/otel-go.git
    cd otel-go
    ```
2. Add your Weather API key on docker compose (To make testing easier, I left my key in the code. After the test correction, I will remove it.)
```sh
    environment:
      - WEATHER_API_KEY=b4f74835750f41c0bfe24936250801
```
3. Run docker compose:
    ```sh
    docker compose up
    ```
4. Use service.http file to test endpoints
    ```http
    ### Service A
    POST http://localhost:8080/weather
    Content-Type: application/json

    {
    "cep": "35780000"
    }

    ### Service B - GET weather by CEP
    GET http://localhost:8081/weather/35780000

    ### Service B - POST weather
    POST http://localhost:8081/weather
    Content-Type: application/json

    {
    "cep": "35780000"
    }
    ```
5. Access zipkin
    ```http
    http://localhost:9411 
    ```
