FROM golang:1.23-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /plex-mcp-go .

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /plex-mcp-go /plex-mcp-go

USER nonroot:nonroot

ENTRYPOINT ["/plex-mcp-go"]
