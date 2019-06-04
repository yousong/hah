hah echoes back to [hey](https://github.com/rakyll/hey)

It's a tiny WebSocket, HTTP, TCP, UDP server capable of listening on multiple
ports echoing back request info.

It was written for testing out loadbalancer setups.
The program was originally known as
[echoserver](https://github.com/yousong/gists/tree/master/python/echoserver)
and written in Python with tornado and some C extention code.
It's rewritten here for easier deployment and better performance.

# Usage

	./hah --http 2080,20443 --tcp 2022 --udp 2053
	./hah --http 2080/proxy=1,20443 --tcp 2022/proxy=1 --udp 2053
	./hah --http 2080,20443/proxy=0 --tcp 2022 --udp 2053/proxy=0 -proxy
