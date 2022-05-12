export GO111MODULE=on
export GOFLAGS=-mod=vendor
export SERVER_PORT=8080
PROJECT_NAME="migrate-leave-krow-to-xero"
APP=migrate-leave-krow-to-xero

update-vendor:
	go mod tidy
	go mod vendor

clean:
	rm -f ${APP}

build: clean
	go build -o ${APP}

container: build
	docker build . -t ${APP}

push:
	docker push ${APP}

test:
	set -o pipefail; go test -v ./... 2>&1 | tee test-output.txt

sonar:
	mkdir -p gen
	go test `go list ./... | grep -vE "./test"` \
	   -race -covermode=atomic -json \
	   -coverprofile=$(COVER_FILE)

test-coverage:
	go test -short -coverprofile cover.out -covermode=atomic ${PKG_LIST}
	cat cover.out >> test-output.txt

.PHONY: \
	clean \
	build \
	test \
	test-coverage \
	container \
	push \
	lint \