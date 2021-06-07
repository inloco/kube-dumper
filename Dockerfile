FROM alpine:3.13

ADD https://s3.amazonaws.com/aws-cli/awscli-bundle.zip awscli-bundle.zip
RUN unzip awscli-bundle.zip && \
    apk add --no-cache python && \
    ./awscli-bundle/install -i /usr/local/aws -b /usr/local/bin/aws && \
    rm -fR awscli-bundle*

ADD https://dl.k8s.io/release/v1.21.1/bin/linux/amd64/kubectl /usr/local/bin/kubectl
RUN chmod +x /usr/local/bin/kubectl

ADD https://github.com/mozilla/sops/releases/download/v3.7.1/sops-v3.7.1.linux /usr/local/bin/sops
RUN chmod +x /usr/local/bin/sops

RUN apk add --no-cache git && \
    git config --global credential.helper '!aws codecommit credential-helper $@' && \
    git config --global credential.UseHttpPath true && \
    git config --global user.email 'kube-dumper@incognia.com' && \
    git config --global user.name 'Kube Dumper'

RUN apk add --no-cache jq ruby ruby-json

COPY ./main.sh /init
RUN chmod +x /init

WORKDIR $HOME/workdir
ENTRYPOINT [ "/init" ]
