FROM alpine:3.12
MAINTAINER Yousong Zhou <yszhou4tech@gmail.com>

ADD hah /

ENTRYPOINT ["/hah"]
CMD [ \
	"--http", "2080,20443", \
	"--tcp", "2022,2023", \
	"--udp", "2053" \
]
