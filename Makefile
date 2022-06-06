SHELL := /bin/bash
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
	set -euxo pipefail; go build -o ${APP}

container: build
	set -euxo pipefail; docker build . -t ${APP}

push:
	docker push ${APP}

test:
	set -euxo pipefail; go test $(shell go list ./... | grep -v /test/blackbox) 2>&1 | tee test-output.txt

sonar:
	mkdir -p gen
	set -euxo pipefail;
	go test `go list ./... | grep -vE "./test"` \
	   -race -covermode=atomic -json \
	   -coverprofile=$(COVER_FILE)

.PHONY: bbtest
bbtest:
	@echo "Running blackbox tests"
	(docker-compose up --force-recreate --always-recreate-deps --abort-on-container-exit --build blackbox) || { docker-compose logs -t; exit 1; }
	docker-compose down

test-coverage:
	set -euxo pipefail;
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