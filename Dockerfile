FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cndsdb ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/cndsdb /cndsdb
USER nonroot:nonroot
VOLUME ["/var/lib/cndsdb"]
# 8080 serves the HTTP API, 7000 carries Raft traffic between nodes.
EXPOSE 8080 7000
ENTRYPOINT ["/cndsdb"]
