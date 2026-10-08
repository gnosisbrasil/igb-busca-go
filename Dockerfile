FROM golang:alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 go build -o /api .

FROM alpine:3.21
WORKDIR /app
COPY --from=build /api /app/api
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -O- http://localhost:${PORT:-3000}/health > /dev/null || exit 1
CMD ["./api"]
