# mine-core is a public module: building needs no registry login, GOPRIVATE or
# git credentials.
FROM golang:1.27-alpine

WORKDIR /app

ENV GOTOOLCHAIN=auto

ADD go.mod go.sum /app/
RUN go mod download

# No @version: air is a tool dependency of this module, so this installs the
# version go.mod pins — the same one `make dev` runs on the host.
RUN go install github.com/air-verse/air

ADD . /app/

# .air.toml is written for every platform: this is linux, so it takes the
# [build] table and builds ./tmp/main — the .exe a Windows host leaves in the
# bind-mounted ./tmp is a different file and never collides.
CMD ["air", "-c", ".air.toml"]
