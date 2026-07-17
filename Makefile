.PHONY: dev down logs test build

dev:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f --tail=200

test:
	cd backend && go test ./...
	cd frontend && npm run typecheck

build:
	docker compose build

