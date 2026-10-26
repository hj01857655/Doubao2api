FROM golang:1.24-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /build
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o doubao2api .

FROM alpine:3.21
RUN apk --no-cache add ca-certificates wget \
    && adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=builder /build/doubao2api .
RUN chown -R root:root /app && chmod -R 555 /app \
    && mkdir -p /app/data && chown appuser:appuser /app/data && chmod 755 /app/data
USER appuser
EXPOSE 8378
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget -qO- http://localhost:8378/health || exit 1
CMD ["./doubao2api"]
