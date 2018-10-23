FROM golang:latest AS golang
ENV GOPATH /go
WORKDIR /go/src/github.com/cisco-sso/snapshotpolicy
RUN go get -u github.com/golang/dep/cmd/dep
COPY . .
RUN ./hack/update-codegen.sh
RUN dep ensure
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o snapshotpolicy .

FROM scratch
COPY --from=golang /go/src/github.com/cisco-sso/snapshotpolicy/snapshotpolicy /
COPY --from=golang /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
EXPOSE 8080
ENTRYPOINT ["/snapshotpolicy"]
