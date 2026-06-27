FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /pixelwave-server ./cmd/server

FROM alpine:3.20
RUN adduser -D -H appuser
COPY --from=build /pixelwave-server /pixelwave-server
USER appuser
EXPOSE 8080
ENTRYPOINT ["/pixelwave-server"]
