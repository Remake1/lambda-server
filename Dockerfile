
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /dist/server ./cmd/server

FROM alpine:latest

RUN adduser -D appuser

WORKDIR /app

COPY --from=builder /dist/server .

EXPOSE 3000

USER appuser

CMD ["./server"]