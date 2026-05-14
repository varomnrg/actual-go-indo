FROM golang:1.23-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o banktoactual .

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    poppler-utils \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /build/banktoactual .

RUN mkdir -p /app/data /app/uploads

EXPOSE 8080

CMD ["./banktoactual"]
