# mine-core is a public module: building needs no registry login, GOPRIVATE or
# git credentials.
FROM golang:1.27-alpine

WORKDIR /app

ENV GOTOOLCHAIN=auto

ADD go.mod go.sum /app/
RUN go mod download
ADD . /app/

RUN go build -o main

FROM alpine:3.22
# Outbound TLS (Sentry, S3, FCM, any HTTPS call) needs the root store; a scratch
# alpine has none.
RUN apk add --no-cache ca-certificates tzdata
COPY --from=0 /app/main /main
CMD ["/main"]
