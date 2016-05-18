build := build
app_name := elastalertRuleLoader
DOCKER_IMAGE_NAME ?= nordstrom/elastalertruleloader
DOCKER_IMAGE_TAG  ?= 1.0.2

.PHONY: build build_image release_image

build: *.go
	docker run --rm \
	  -e CGO_ENABLED=true \
	  -e OUTPUT=make buil$(app_name) \
	  -v $(shell pwd):/src \
	  centurylink/golang-builder

build_image: Dockerfile
	@echo ">> building docker image"
	docker build -t "$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)" .

release_image:
	@echo ">> push docker image"
	@docker push "$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)"
