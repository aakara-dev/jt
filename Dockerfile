# Usage:
#   $ docker build -t jt:latest .
#   $ docker run --rm -it -v ~/.netrc:/root/.netrc -v ~/.config/jt:/root/.config/jt jt

FROM golang:1.26-alpine3.24 AS builder

ENV CGO_ENABLED=0
ENV GOOS=linux

WORKDIR /app

COPY . .

RUN set -eux; \
    env ; \
    ls -la ; \
    apk add -U --no-cache make git ; \
    make deps install

FROM alpine:3.24

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /go/bin/jt /bin/jt

ENTRYPOINT ["/bin/jt"]
