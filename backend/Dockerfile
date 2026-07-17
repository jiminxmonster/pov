FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/pov-api ./cmd/api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/pov-api /app/pov-api
RUN mkdir -p /data/uploads && chown -R 65532:65532 /data
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/app/pov-api"]
