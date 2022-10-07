FROM alpine:3.16

ADD https://dl.k8s.io/release/v1.25.2/bin/linux/amd64/kubectl /usr/local/bin/kubectl
RUN chmod +x /usr/local/bin/kubectl

ADD https://github.com/mozilla/sops/releases/download/v3.7.3/sops-v3.7.3.linux.amd64 /usr/local/bin/sops
RUN chmod +x /usr/local/bin/sops

RUN apk add --no-cache aws-cli bash git jq parallel py3-pip && \
    pip install yq && \
    git config --global credential.helper '!aws codecommit credential-helper $@' && \
    git config --global credential.UseHttpPath true && \
    git config --global user.email 'kube-dumper@incognia.com' && \
    git config --global user.name 'Kube Dumper' && \
    mkdir ~/.parallel && \
    touch ~/.parallel/will-cite && \
    mkdir ~/workdir && \
    unlink /sbin/init

COPY ./main.sh /sbin/init
RUN chmod +x /sbin/init

WORKDIR ~/workdir
ENTRYPOINT ["/sbin/init"]
