ARG SRCDIR=/go/src/github.com/otoolep/hraftd

FROM golang:1.16.1 AS build
ARG SRCDIR
WORKDIR ${SRCDIR}
ADD http http
ADD store store
ADD metrics metrics
ADD main.go go.mod go.sum ./
RUN go build -o hraftd .

FROM golang:1.16.1
ARG SRCDIR
WORKDIR /opt/hraftd
COPY --from=build ${SRCDIR}/hraftd .
