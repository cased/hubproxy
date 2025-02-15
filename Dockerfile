FROM golang:1.21-alpine

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /hubproxy ./cmd/hubproxy

EXPOSE 8080 8081
CMD ["/hubproxy"]
