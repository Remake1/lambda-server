
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /dist/server./cmd/server

FROM alpine:latest

RUN adduser -D appuser

WORKDIR /app

COPY --from=builder /dist/server .

EXPOSE 3000

USER appuser

CMD ["./server"]