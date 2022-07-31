FROM golang:1.10.3 as build
COPY . /go/src/github.com/BobZombiE69/btcpool-go-modules/
RUN cd /go/src/github.com/BobZombiE69/btcpool-go-modules/userChainAPIServer && go build

FROM php:7.4-cli
COPY --from=build /go/src/github.com/BobZombiE69/btcpool-go-modules/userChainAPIServer/userChainAPIServer /usr/local/bin/
COPY install/cfg-generator/ /usr/local/bin/
COPY userChainAPIServer/entrypoint.sh /

ENTRYPOINT ["/entrypoint.sh"]
