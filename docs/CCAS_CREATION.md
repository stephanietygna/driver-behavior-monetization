# Tutorial de criação de um chaincode externo

Primeiro, certifique-se de que o seu chaincode está configurado para realizar conexões TLS, assim como, por exemplo, no chaincode [assetTransfer](../chaincode-external/asset-external/assetTransfer.go). Repare as funções main e getTLSProperties.

Depois, crie um Dockerfile com estrutura similar a seguinte


```bash
ARG GO_VER=1.21.7
ARG ALPINE_VER=3.18

FROM golang:${GO_VER}-alpine${ALPINE_VER}



WORKDIR /go/src/github.com/seu/repositorio/nomedochaincode
COPY . .

RUN go get -d -v ./...
RUN go install -v ./...

EXPOSE 9999

CMD ["nomedochaincode"]

```

E faça o push para seu perfil no Docker

```bash
docker build -t <username>/chaincode .
docker push <username>/chaincode
```

Agora, o chaincode externo está pronto para ser [instalado](../chaincode-external/)