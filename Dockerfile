# syntax=docker/dockerfile:1

FROM golang:1.23-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ai-api-stronger ./cmd

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/ai-api-stronger /app/ai-api-stronger
COPY config.example.yaml /app/config.yaml

ENV CONFIG_PATH=/app/config.yaml
ENV ENV_PATH=

EXPOSE 8888

USER nonroot:nonroot

ENTRYPOINT ["/app/ai-api-stronger"]
CMD ["-config", "/app/config.yaml"]
