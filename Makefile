

clean::
	rm ./bin/*

ensure::
	go mod download

build:: ensure
	mkdir ./bin
	go build -o ./bin ./cmd/...

# Ensure that in tests, the language server is accessible
test:: build
	PATH="${PWD}/bin:${PATH}" go test --timeout 10m ./pkg/...
