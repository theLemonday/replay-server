.PHONY: build run tidy gen smoke

gen:
	oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml

tidy:
	go mod tidy

build: tidy
	go build -o bin/dynserver .

run: build
	./bin/dynserver -config dynserver.yaml

smoke:
	@echo "==> register sub-server on :9000"
	curl -sf -X POST http://localhost:8080/servers \
	  -H 'Content-Type: application/json' \
	  -d '{"port":9000,"name":"demo"}' | jq .

	@echo "==> register GET /hello on port 9000"
	curl -sf -X POST http://localhost:8080/servers/9000/routes \
	  -H 'Content-Type: application/json' \
	  -d '{"method":"GET","path":"/hello","response":{"status_code":200,"headers":{"Content-Type":"application/json"},"body":"{\"msg\":\"hello\"}"}}' | jq .

	@echo "==> hit registered route"
	curl -si http://localhost:9000/hello

	@echo "==> hit unregistered route (expect 200 empty)"
	curl -si http://localhost:9000/unknown
