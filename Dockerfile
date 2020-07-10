FROM golang:alpine AS builder
RUN mkdir -p /build
ADD . /build
WORKDIR /build
RUN go build -o hah .


FROM alpine:3.12
MAINTAINER Yousong Zhou <yszhou4tech@gmail.com>

EXPOSE \
	2080/tcp \
	20443/tcp \
	2022/tcp \
	2023/tcp \
	2053/udp

COPY --from=builder /build/hah /

ENTRYPOINT ["/hah"]
CMD [ \
	"--http", "2080,20443", \
	"--tcp", "2022,2023", \
	"--udp", "2053" \
]
