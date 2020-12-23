FROM golang:1.15 AS build
WORKDIR /go/src/github.com/inloco/kube-dumper
COPY ./go.mod ./go.sum ./
RUN go mod download
COPY ./*.go ./
RUN CGO_ENABLED=0 go install -a -v -trimpath -ldflags '-d -extldflags "-fno-PIC -static"' -tags 'netgo osusergo static_build' ./...

FROM alpine:3.12 AS runtime
ADD https://github.com/mozilla/sops/releases/download/v3.5.0/sops-v3.5.0.linux /usr/local/bin/sops
RUN chmod +x /usr/local/bin/sops 

RUN apk add --no-cache openssh-client git && \
    mkdir ~/.ssh && \
    ssh-keyscan -t rsa github.com >> ~/.ssh/known_hosts && \
    git config --global user.email "kube-dumper@inloco.com.br" && \
    git config --global user.name "Kube Dumper"

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /go/bin/kube-dumper /init

WORKDIR /root/dumper
CMD [ "/init" ]
