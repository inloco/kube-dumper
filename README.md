# kube-dumper

K8s Backup Service that listens to K8s events and automatically updates K8s backup.

## Configuration

### Environment Variables

```
REPOSITORY_URL=git@github.com:mvgmb/kube-dumper-test.git
REFRESH_GVRS_TIME_IN_MINUTES=1
```

### K8s

Since it's expected to be running inside a pod running on K8s, by default this program tries to use the service account K8s gives to pods to generate a configuration.

If it fails, it'll try picking up the default file used by K8s (`~/.kube/config`).

### SOPS

This program uses SOPS to encrypt secret configuration files.

It uses the creation rules defined in `.sops.yaml` configuration file during encryption. Here's an example of a SOPS configuration using AWS KMS:

```
creation_rules:
  - encrypted_regex: ^(data|stringData)$
    kms: arn:aws:kms:us-east-2:466238317701:key/463d7832-ad9c-4bef-aff3-55ae1151ad4e

```

To enforce `git diff` decrypts **_secrets_** before diffing, _textconv_ option must be configured in `.gitconfig` file:

```
[diff "sopsdiffer"]
	textconv = sops -d
```

and `.gitattribute` file:

```
**/secrets/*.yaml diff=sopsdiffer
```

These files must be placed in dumper's git repository. Here's an example of a dumper https://github.com/mvgmb/kube-dumper-test.

Read more: https://github.com/mozilla/sops

### AWS KMS

When using AWS KSM, you'll need your AWS credentials to authenticate with AWS services. This program picks up the credentials from AWS SDK's default credential chain. The common items in the credential chain are the following:

- Environment Credentials
- Shared Credentials file (`~/.aws/credentials`)
- EC2 Instance Role Credentials

Read more: https://github.com/aws/aws-sdk-go#configuring-credentials

### Field Filters

This program uses `./dump-files/fieldFilters.yaml` fields to filter undesired YAML fields.

## Usage

### Locally

Prerequisites:

- [Go](https://golang.org/doc/install) v1.15.5

> WARNING: running this code will delete all content from current folder

```bash
# create an empty directory
mkdir tmp
cd ./tmp

# load environment variables
source ../env.sh

# run program
go run ../*.go
```

### On K8s Cluster

Prerequisites:

- [docker](https://docs.docker.com/get-docker/) v20.10.0
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) v1.19.4
- [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/) v3.9.0
- [SOPS Kustomize Generator Plugin](https://github.com/inloco/sops-kustomize-generator-plugin) v1.1.1

> To learn how to generate encrypted `aws.secret.yaml` and `ssh.secret.yaml` files look into https://github.com/inloco/sops-kustomize-generator-plugin

To build service's docker image, run:

```bash
docker build . -t inloco/kube-dumper
```

This repository uses Kustomize to generate K8s configuration files. To apply to K8s run:

```bash
kustomize build --enable_alpha_plugins ./k8s | kubectl apply -f -
```

## Contributors

https://github.com/inloco/kube-dumper/contributors
