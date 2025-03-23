FROM golang:1.23-alpine as builder

WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o svc-a cmd/api/main.go

FROM alpine:3.21.3
WORKDIR /app
COPY --from=builder /app/svc-a .

EXPOSE 8080

CMD ["./svc-a"]