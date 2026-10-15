FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/cndsdb ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/cndsdb /cndsdb
VOLUME ["/var/lib/cndsdb"]
EXPOSE 8080
ENTRYPOINT ["/cndsdb"]