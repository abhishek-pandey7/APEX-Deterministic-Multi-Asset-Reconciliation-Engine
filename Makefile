.PHONY: test race vet check test-int load run serve web worker ai-setup eval gen gen-test db-up db-down db-reset db-test psql

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

check: vet race gen-test

DB_URL ?= postgres://apex:apex@localhost:55432/apex?sslmode=disable

# Python virtualenvs keep executables in Scripts/ on Windows and bin/ elsewhere.
ifeq ($(OS),Windows_NT)
VENV_PY := tools/.venv/Scripts/python
else
VENV_PY := tools/.venv/bin/python
endif

test-int:
	APEX_TEST_DATA=$(CURDIR)/data/small APEX_TEST_DB_URL=$(DB_URL) go test ./...

run:
	go run ./cmd/apex run

serve:
	go run ./cmd/apex serve

web:
	cd web && npm ci && npm run build

ai-setup:
	python -m venv tools/.venv && $(VENV_PY) -m pip install -r tools/apexai/requirements.txt

worker:
	cd tools && ../$(VENV_PY) -m apexai worker

eval:
	go run ./cmd/apex eval -data data/small

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
