FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/kmjg-hub-server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/kmjg-hub-server /usr/local/bin/kmjg-hub-server
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/kmjg-hub-server"]
