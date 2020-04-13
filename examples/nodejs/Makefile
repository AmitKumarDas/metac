tag = 0.2

image:
	docker build -t metacontroller/nodejs-server:$(tag) .

push: image
	docker push metacontroller/nodejs-server:$(tag)
