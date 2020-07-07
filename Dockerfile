FROM alpine:3.12
MAINTAINER Yousong Zhou <yszhou4tech@gmail.com>

EXPOSE \
	2080/tcp \
	20443/tcp \
	2022/tcp \
	2023/tcp \
	2053/udp

ADD hah /

ENTRYPOINT ["/hah"]
CMD [ \
	"--http", "2080,20443", \
	"--tcp", "2022,2023", \
	"--udp", "2053" \
]
