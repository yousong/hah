TOPDIR:=$(CURDIR)

all: build
.PHONY: all

build:
	CGO_ENABLED=0 go build -tags 'netgo' -ldflags '-extldflags "-static"' .
.PHONY: build

checkVersion=echo "$(VERSION)" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$$'
release:
	$(checkVersion)
	git tag --force "$(VERSION)" HEAD
	git push origin HEAD --tags

docker-image:
	$(checkVersion)
	docker buildx build \
		-f Dockerfile \
		--platform linux/amd64,linux/arm64 \
		--tag "yousong/hah:$(VERSION)" \
		--push \
		.

.PHONY: docker-image
