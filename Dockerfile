FROM golang:1.27.4-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG SERVICE
RUN CGO_ENABLED=0 go build -trimpath -o /out/service ./cmd/${SERVICE}

FROM debian:bookworm-slim
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN mkdir -p /data && chown 10001:10001 /data
USER 10001:10001
WORKDIR /app
COPY --from=build /out/service /app/service
ENTRYPOINT ["/app/service"]
