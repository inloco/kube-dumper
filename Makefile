IMAGE_NAME ?= inloco/kube-dumper
IMAGE_VERSION ?= v1.1.1

docker: docker-build docker-push

docker-build: docker-build
	docker build -t $(IMAGE_NAME):$(IMAGE_VERSION) .

docker-push:
	docker push $(IMAGE_NAME):$(IMAGE_VERSION)
