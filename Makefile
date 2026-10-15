.PHONY: test race vet check db-up db-down db-reset db-test psql

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

check: vet race

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
