FROM golang:1.10.3  as build

# dependencies
RUN go get -v github.com/segmentio/kafka-go \
 && go get -v github.com/golang/snappy \
 && go get -v github.com/go-sql-driver/mysql \
 && go get -v github.com/golang/glog

COPY . /go/src/github.com/BobZombiE69/btcpool-go-modules/
RUN bash /go/src/github.com/BobZombiE69/btcpool-go-modules/chainSwitcher/build.sh

FROM php:7.4-cli
COPY --from=build /go/src/github.com/BobZombiE69/btcpool-go-modules/chainSwitcher/chainSwitcher /usr/local/bin/
COPY install/cfg-generator/ /usr/local/bin/
COPY chainSwitcher/entrypoint.sh /

ENTRYPOINT ["/entrypoint.sh"]
