FROM golang:1.16-alpine3.12

RUN apk update && apk upgrade
RUN apk add --no-cache gcc musl-dev git npm shfmt shellcheck

RUN go get -v golang.org/x/tools/cmd/goimports@v0.1.8
RUN go get -u github.com/client9/misspell/cmd/misspell
RUN go get -v github.com/golangci/golangci-lint/cmd/golangci-lint@v1.43.0

RUN go get -v github.com/boumenot/gocover-cobertura
RUN go get -v github.com/jstemmer/go-junit-report


RUN npm install -g npm

RUN npm install -g prettier prettier-plugin-go-template browserslist

RUN npm install -g jest jest-junit

RUN npm install -g eslint eslint-plugin-compat eslint-plugin-jest \
	eslint-plugin-promise eslint-plugin-unicorn

RUN npm install -g stylelint stylelint-config-prettier \
	stylelint-config-rational-order stylelint-config-standard \
	stylelint-no-unsupported-browser-features stylelint-order
