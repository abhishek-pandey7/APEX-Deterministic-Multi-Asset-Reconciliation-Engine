.PHONY: test race vet check test-int load gen gen-test db-up db-down db-reset db-test psql

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

check: vet race gen-test

DB_URL ?= postgres://apex:apex@localhost:55432/apex?sslmode=disable

test-int:
	APEX_TEST_DATA=$(CURDIR)/data/small APEX_TEST_DB_URL=$(DB_URL) go test ./...

load:
	go run ./cmd/apex ingest -layout apex-ledger-cash data/small/ledger_cash.csv
	go run ./cmd/apex ingest -layout custodian-cash-v1 data/small/custodian_cash.csv
	go run ./cmd/apex ingest -layout apex-ledger-positions data/small/ledger_positions.csv
	go run ./cmd/apex ingest -layout custodian-positions-v1 data/small/custodian_positions.csv

gen:
	cd tools && python -m apexgen --out ../data/small --seed 42

gen-test:
	cd tools && python -m unittest discover -s apexgen/tests -t .

db-up:
	docker compose up -d db
	docker compose run --rm migrate

db-down:
	docker compose down

db-reset:
	docker compose down -v
	$(MAKE) db-up

psql:
	docker compose exec db psql -U apex -d apex

db-test:
	docker compose exec -T db psql -U apex -d apex -q -v ON_ERROR_STOP=1 < tests/sql/break_audit_test.sql
