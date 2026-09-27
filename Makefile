# DFSha — atajos de construcción y verificación.

MODULE := github.com/thomaszambrano/DFS-Topicos-Telematica
GOBIN  := $(shell go env GOPATH)/bin
export PATH := $(PATH):$(GOBIN)

.DEFAULT_GOAL := help

.PHONY: help
help: ## Muestra esta ayuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: tools
tools: ## Instala los plugins de protoc
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

.PHONY: proto
proto: ## Regenera el código gRPC desde api/dfsha.proto
	protoc --go_out=. --go_opt=module=$(MODULE) \
	       --go-grpc_out=. --go-grpc_opt=module=$(MODULE) \
	       api/dfsha.proto
	@echo "generado internal/pb/"

.PHONY: build
build: ## Compila los dos binarios
	go build -o bin/controlnode ./cmd/controlnode
	go build -o bin/datanode ./cmd/datanode
	@echo "binarios en bin/"

.PHONY: fmt
fmt: ## Formatea el código Go
	gofmt -w .

.PHONY: vet
vet: ## Análisis estático
	go vet ./...

.PHONY: test
test: ## Corre las pruebas
	go test ./...

.PHONY: up
up: ## Levanta el clúster: 1 ControlNode y 4 DataNodes
	docker compose up -d --build
	@echo "esperando a que el clúster reporte..."
	@sleep 6
	@curl -s http://localhost:8080/v1/nodes | head -c 2000; echo

.PHONY: down
down: ## Detiene el clúster (conserva los volúmenes)
	docker compose down

.PHONY: clean
clean: ## Detiene el clúster y BORRA los volúmenes
	docker compose down -v
	rm -rf bin

.PHONY: logs
logs: ## Sigue los logs del clúster
	docker compose logs -f

.PHONY: verify
verify: ## Ejecuta la verificación completa del hito (E.2)
	./scripts/verificar.sh

.PHONY: shell
shell: ## Abre la consola del cliente
	cd client && python3 -m dfsha shell
