FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cndsdb ./cmd/server && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/cndsdb /cndsdb
# Pre-create the data dir as nonroot so new Docker volumes inherit writable ownership.
COPY --from=build --chown=65532:65532 /out/data /var/lib/cndsdb
USER nonroot:nonroot
VOLUME ["/var/lib/cndsdb"]
# 8080 serves the HTTP API, 7000 carries Raft traffic between nodes.
EXPOSE 8080 7000
ENTRYPOINT ["/cndsdb"]
