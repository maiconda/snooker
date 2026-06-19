FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY lobby ./lobby

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/lobby-service ./lobby/cmd/lobby

FROM alpine:3.22

RUN apk --no-cache add ca-certificates curl

WORKDIR /app

COPY --from=builder /out/lobby-service .

EXPOSE 8080

CMD ["./lobby-service"]
