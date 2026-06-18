FROM golang:1.26.4-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/marcajes-api ./cmd/marcajes-api

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/marcajes-api /app/marcajes-api

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/app/marcajes-api"]
