FROM golang:1.25.4-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/bark-secure-proxy ./cmd/bark-secure-proxy

FROM alpine:3.20

RUN addgroup -S bark && adduser -S bark -G bark

WORKDIR /app
COPY --from=build /out/bark-secure-proxy /app/bark-secure-proxy
COPY --from=build /src/web /app/web
COPY --from=build /src/config.example.yaml /app/config.yaml

RUN mkdir -p /app/data && chown -R bark:bark /app
USER bark

EXPOSE 8090
VOLUME ["/app/data"]

ENTRYPOINT ["./bark-secure-proxy","-config","/app/config.yaml"]
