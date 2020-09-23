BINARIES=bin/deadman-switch
IMAGE=containers.trusch.io/deadman-switch:latest
BASE_IMAGE=gcr.io/distroless/base-debian10:latest
BUILD_IMAGE=golang:1.15

GOOS=linux
GOARCH=amd64
GOARM=7 # for crosscompiling

COMMIT=$(shell git log --format="%H" -n 1)
VERSION=$(shell git describe)

default: image

run: image
	podman pod create -p 8080:8080 --name deadman-switch-pod --replace
	podman run -d --rm --pod deadman-switch-pod --name etcd quay.io/coreos/etcd
	podman run -d --rm --pod deadman-switch-pod --name caddy -v ./configs/Caddyfile:/Caddyfile caddy caddy run -config /Caddyfile
	podman run -d --rm --pod deadman-switch-pod --name deadman-switch-1 -v ./configs/config-node-1.yaml:/config.yaml $(IMAGE) \
		/bin/deadman-switch -c /config.yaml --log-level info --log-format console
	podman run -d --rm --pod deadman-switch-pod --name deadman-switch-2 -v ./configs/config-node-2.yaml:/config.yaml $(IMAGE) \
		/bin/deadman-switch -c /config.yaml --log-level info --log-format console

stop:
	podman pod rm -f deadman-switch-pod

# put binaries into image
image: .image
.image: $(BINARIES) Makefile
	$(eval ID=$(shell buildah from $(BASE_IMAGE)))
	buildah copy $(ID) $(shell pwd)/bin/* /bin/
	buildah commit $(ID) $(IMAGE)
	buildah rm $(ID)
	touch .image

# build binaries
bin/%: $(shell find ./ -name "*.go")
	podman run \
		--rm \
		-v ./:/app \
		-w /app \
		-e GOOS=${GOOS} \
		-e GOARCH=${GOARCH} \
		-e GOARM=${GOARM} \
		-v go-build-cache:/root/.cache/go-build \
		-v go-mod-cache:/go/pkg/mod $(BUILD_IMAGE) \
			go build -v -o $@ -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT)" cmd/deadman-switch/main.go

# install locally
install: ${BINARIES}
	cp -v ${BINARIES} $(shell go env GOPATH)/bin/

# cleanup
clean:
	-rm -r bin .image .buildimage /tmp/protoc-download
	-podman volume rm  go-build-cache go-mod-cache
