build := build
app_name := elastalertRuleLoader
DOCKER_IMAGE_NAME ?= nordstrom/elastalertRuleLoader
DOCKER_IMAGE_TAG  ?= 1.0.2

.PHONY: build_image release_image

$(build)/$(app_name): 
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $@

build_image: Dockerfile $(build)/$(app_name)
	@echo ">> building docker image"
	cp Dockerfile $(build)
	@docker build -t "$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)" $(build)

release_image:
	@echo ">> push docker image"
	@docker push "$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)"

