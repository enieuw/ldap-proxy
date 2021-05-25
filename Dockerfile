FROM golang:alpine AS builder
RUN apk update && apk add --no-cache git
WORKDIR $GOPATH/github.com/enieuw/ldap-proxy
COPY . .
RUN go get -d -v
RUN CGO_ENABLED=0 go build -o /go/bin/ldap-proxy

FROM scratch
COPY --from=builder /go/bin/ldap-proxy /go/bin/ldap-proxy
ENTRYPOINT ["/go/bin/ldap-proxy"]
