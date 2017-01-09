container_name := elastalertruleloader
container_registry := quay.io/nordstrom
container_release := 1.0.5

app_name := elastalertRuleLoader

.PHONY: build/image tag/image push/image

$(app_name): *.go
	docker run --rm \
	  -e CGO_ENABLED=true \
	  -e OUTPUT=$(app_name) \
	  -v $(shell pwd):/src \
	  centurylink/golang-builder

	chmod 0755 $(app_name)

build/image: $(app_name) Dockerfile
	docker build \
		-t $(container_name) .

tag/image: build/image
	docker tag $(container_name) $(container_registry)/$(container_name):$(container_release)

push/image: tag/image
	docker push $(container_registry)/$(container_name):$(container_release)

release: build/image push/image