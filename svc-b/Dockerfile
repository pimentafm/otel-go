FROM golang:1.23-alpine as builder

WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o svc-b cmd/api/main.go

FROM alpine:3.21.3
WORKDIR /app
COPY --from=builder /app/svc-b .

EXPOSE 8081

CMD ["./svc-b"]