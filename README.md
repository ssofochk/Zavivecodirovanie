## PG Setup

Требования: PG 15

Создание БД

```sql
CREATE DATABASE itmo_ledger;
```

Создание роли

```sql
CREATE ROLE itmo_ledger WITH LOGIN PASSWORD 'Secret123';
```

Передать права новому пользователю 

```sql
ALTER DATABASE itmo_ledger OWNER TO itmo_ledger;
```

Connection string подхватывается из переменной окружения DB_DSN или из флага cli db-dsn

```bash
DB_DSN=postgres://itmo_ledger:Secret123@localhost/itmo_ledger?sslmode=disable
```

Накатить миграции

```bash
migrate -path=./migrations -database=$DB_DSN up
```

## Запуск

ВАЖНО: Установить DB_DSN, см выше

```bash
go run ./cmd/api
```

## Примеры запросов

Добавление/создание баланса
```bash
curl -X POST localhost:8080/v1/transactions -d '{"user_id": "653F535D-10BA-4186-A05B-74493354F13B", "amount": 100, "type": "deposit"}' 
```

Списание средств
```bash
curl -X POST localhost:8080/v1/transactions -d '{"user_id": "653F535D-10BA-4186-A05B-74493354F13B", "amount": 200, "type": "withdrawal"}' 
```

Получение баланса
```bash
curl -X GET localhost:8080/v1/users/653F535D-10BA-4186-A05B-74493354F13B/balance 
```