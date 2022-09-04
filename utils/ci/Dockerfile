FROM golang:1.18-alpine3.15

#RUN apk update && apk upgrade
RUN apk add --no-cache gcc musl-dev git npm shellcheck

RUN go install mvdan.cc/sh/v3/cmd/shfmt@v3.5.1 && rm -r /go/pkg/*
RUN go install golang.org/x/tools/cmd/goimports@v0.1.12 && rm -r /go/pkg/*
RUN go install github.com/client9/misspell/cmd/misspell@v0.3.4 && rm -r /go/pkg/*
RUN go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.49.0 && rm -r /go/pkg/*

RUN go install github.com/boumenot/gocover-cobertura@v1.2.0 && rm -r /go/pkg/*
RUN go install github.com/jstemmer/go-junit-report@v1.0.0 && rm -r /go/pkg/*
