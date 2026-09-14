ARG GO_VERSION=1.23

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /app

# Primeiro as dependências: isso acelera novas compilações da imagem.
COPY go.mod ./
RUN go mod download

COPY . .
# Gera/atualiza go.sum no ambiente isolado de compilação antes de construir.
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /risk-factor-chaincode .

FROM alpine:3.20
COPY --from=build /risk-factor-chaincode /risk-factor-chaincode
EXPOSE 9999
ENTRYPOINT ["/risk-factor-chaincode"]
