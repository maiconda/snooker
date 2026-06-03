# Stage 1: Build da aplicação Go
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copia arquivos de dependências
COPY go.mod ./
# COPY go.sum ./ (descomente quando possuir dependências)
RUN go mod download

# Copia o restante do código
COPY . .

# Compila o binário estático
RUN CGO_ENABLED=0 GOOS=linux go build -o snooker-api main.go

# Stage 2: Container final leve
FROM alpine:latest

RUN apk --no-cache add ca-certificates curl

WORKDIR /app

# Copia o binário compilado no Stage 1
COPY --from=builder /app/snooker-api .

# Expõe a porta de execução
EXPOSE 8080

# Executa a aplicação
CMD ["./snooker-api"]
