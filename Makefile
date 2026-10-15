.PHONY: test race vet check gen gen-test db-up db-down db-reset db-test psql

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

check: vet race gen-test

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
