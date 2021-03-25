run: swag
	CGO_ENABLED=1 go run -tags=jsoniter .

build: swag
	CGO_ENABLED=1 go build -o build/aur-ci-controller -tags=jsoniter .

deps:
	go get -u github.com/swaggo/swag/cmd/swag

swag: */*.go
	swag init