FROM golang:1.26-alpine AS build

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/miletos-engine ./cmd/engine

FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S miletos && adduser -S -G miletos miletos \
    && mkdir -p /app/input \
    && chown -R miletos:miletos /app
WORKDIR /app
COPY --from=build --chown=miletos:miletos /out/miletos-engine /app/miletos-engine

USER miletos
EXPOSE 8081 9091
ENTRYPOINT ["/app/miletos-engine"]
