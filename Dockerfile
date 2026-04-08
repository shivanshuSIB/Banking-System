FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o banking-server ./cmd/server

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/banking-server .
COPY frontend/ ./frontend/
EXPOSE 8080
CMD ["./banking-server"]
