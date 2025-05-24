.PHONY: migrate up down restart

migrate:
	migrate -database "postgres://your_db_user:your_db_password@localhost:5432/teacher_bot_dev?sslmode=disable" -path migrations up 

up:
	@docker-compose up -d;

down:
	@docker-compose stop;

restart:
	@make -s down;
	@make -s up;