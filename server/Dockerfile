FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/kmjg-hub-server ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S kmjg && adduser -S -G kmjg kmjg \
    && mkdir -p /var/lib/kmjg-hub/files && chown -R kmjg:kmjg /var/lib/kmjg-hub
COPY --from=build /out/kmjg-hub-server /usr/local/bin/kmjg-hub-server
USER kmjg
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/kmjg-hub-server"]
