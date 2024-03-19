PROJECT := unwindia_tmt2

.PHONY: \
	build \
	test
run:
	go run ./cmd/$(PROJECT)/main.go

build:
	go build ./...

test:
	go test -v -count=1 -race ./...

docker:                                                                                                       .
	docker buildx build -t ghcr.io/gsh-lan/$(PROJECT):latest . --platform=linux/amd64

docker-arm:
	docker buildx build -t ghcr.io/gsh-lan/$(PROJECT):latest . --platform=linux/arm64

dockerx:
	docker buildx create --name $(PROJECT)-builder --use --bootstrap
	docker buildx build -t ghcr.io/gsh-lan/$(PROJECT):latest --platform=linux/arm64,linux/amd64 .
	docker buildx rm $(PROJECT)-builder

dockerx-builder:
	docker buildx create --name $(PROJECT)-builder --use --bootstrap

tmt2-client-stubs:
    (curl -o /tmp/swagger.json https://github.com/JensForstmann/tmt2/blob/main/backend/swagger.json
    go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest
    oapi-codegen -package tmt2 /tmp/swagger.json > src/tmt2/client.gen.go)