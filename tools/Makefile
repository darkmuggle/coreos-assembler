PREFIX ?= /usr
DESTDIR ?=
toolsLong=$(shell cd .. && $ git log -n 1 --date=short --pretty=format:%H -- tools)
toolsShort=$(shell cd .. && $ git log -n 1 --date=short --pretty=format:%h -- tools)
toolsVersion=$(shell cd .. && $ git log -n 1 --date=short --pretty=format:%cs~%h -- tools)
minioFlags := -X github.com/minio/minio/cmd.Version=$(toolsVersion) -X github.com/minio/minio/cmd.ReleaseTag=COSA-$(toolsVersion) -X github.com/minio/minio/cmd.CommitID=cosa-$(toolsLong) -X github.com/minio/minio/cmd.ShortCommitID=cosa-$(toolsShort)

export CGO_ENABLED=0
build:
	mkdir -p bin
	test -e bin/golangci-lint || \
	go build -o bin/golangci-lint ./vendor/github.com/golangci/golangci-lint/cmd/golangci-lint
	test -e bin/gosec || \
	go build -o bin/gosec ./vendor/github.com/securego/gosec/cmd/gosec
	test -f bin/schematyper || \
	go build -o bin/schematyper ./vendor/github.com/idubinskiy/schematyper
	test -e bin/gomarkdoc || \
	go build -o bin/gomarkdoc ./vendor/github.com/princjef/gomarkdoc/cmd/gomarkdoc
	test -e bin/minio || \
	(DIR=$$(pwd) && \
	cd ./vendor/github.com/minio/minio && \
	GOOS=linux GO111MODULE=on CGO_ENABLED=0 \
	go build -tags kqueue -trimpath --ldflags "$(minioFlags)" -o $$DIR/bin/minio . )

clean:
	rm -rfv bin

install: bin/minio bin/golangci-lint bin/gosec bin/schematyper bin/gomarkdoc
	install -v -D -t $(DESTDIR)$(PREFIX)/bin bin/{minio,golangci-lint,gosec}

go-deps:
	go mod tidy
	go mod download
	go mod vendor

.PHONY:
schema:
	./generate-schema.sh

