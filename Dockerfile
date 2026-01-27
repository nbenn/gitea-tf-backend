FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o gitea-tf-backend .

# Final image
FROM alpine:3.20

RUN apk --no-cache add ca-certificates

COPY --from=builder /app/gitea-tf-backend /usr/local/bin/

EXPOSE 8080

ENTRYPOINT ["gitea-tf-backend"]
