DATA ?= data/queries.csv
DB   ?= data/typeahead.db

.PHONY: tidy seed load run build test clean
tidy:  ; go mod tidy
seed:  ; go run ./cmd/gen -n 150000 -out $(DATA) && go run ./cmd/load -data $(DATA) -db $(DB)
load:  ; go run ./cmd/load -data $(DATA) -db $(DB)
run:   ; go run ./cmd/server -db $(DB)
build: ; go build -o bin/server ./cmd/server
test:  ; go test ./...
clean: ; rm -f $(DB) $(DB)-shm $(DB)-wal; rm -rf bin
