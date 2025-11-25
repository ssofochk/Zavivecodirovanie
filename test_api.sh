#!/bin/bash

# Скрипт для быстрого тестирования API сервиса бонусных баллов

set -e

# Цвета для вывода
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Конфигурация
API_URL="${API_URL:-http://localhost:8080}"
USER_ID="${USER_ID:-653F535D-10BA-4186-A05B-74493354F13B}"

echo -e "${BLUE}=== Тестирование API сервиса бонусных баллов ===${NC}\n"
echo -e "API URL: ${YELLOW}$API_URL${NC}"
echo -e "User ID: ${YELLOW}$USER_ID${NC}\n"

# Функция для красивого вывода
print_step() {
    echo -e "${GREEN}[$1]${NC} $2"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Функция для HTTP запросов с красивым выводом
make_request() {
    local method=$1
    local endpoint=$2
    local data=$3

    print_step "REQUEST" "$method $endpoint"

    if [ -z "$data" ]; then
        response=$(curl -s -w "\n%{http_code}" "$API_URL$endpoint")
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" "$API_URL$endpoint" \
            -H "Content-Type: application/json" \
            -d "$data")
    fi

    http_code=$(echo "$response" | tail -n 1)
    body=$(echo "$response" | sed '$d')

    if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        echo -e "${GREEN}HTTP $http_code${NC}"
        echo "$body" | jq '.' 2>/dev/null || echo "$body"
    else
        echo -e "${RED}HTTP $http_code${NC}"
        echo "$body" | jq '.' 2>/dev/null || echo "$body"
    fi

    echo ""
}

# Проверка что сервис запущен
print_step "INFO" "Проверка доступности сервиса..."
if ! curl -s -f "$API_URL/v1/points/$USER_ID/balance" > /dev/null 2>&1; then
    print_error "Сервис недоступен на $API_URL"
    print_error "Убедитесь что сервис запущен: go run ./cmd/api"
    exit 1
fi
echo -e "${GREEN}✓ Сервис доступен${NC}\n"

# Тест 1: Добавление баллов
print_step "TEST 1" "Добавление 100 баллов со сроком 30 дней"
make_request "POST" "/v1/points/add" "{
    \"user_id\": \"$USER_ID\",
    \"amount\": 100,
    \"ttl_days\": 30
}"

# Тест 2: Проверка баланса
print_step "TEST 2" "Проверка баланса"
make_request "GET" "/v1/points/$USER_ID/balance"

# Тест 3: Добавление еще баллов
print_step "TEST 3" "Добавление 50 баллов со сроком 7 дней"
make_request "POST" "/v1/points/add" "{
    \"user_id\": \"$USER_ID\",
    \"amount\": 50,
    \"ttl_days\": 7
}"

# Тест 4: Проверка баланса
print_step "TEST 4" "Проверка баланса после второго начисления"
make_request "GET" "/v1/points/$USER_ID/balance"

# Тест 5: Списание баллов
print_step "TEST 5" "Списание 30 баллов"
make_request "POST" "/v1/points/withdraw" "{
    \"user_id\": \"$USER_ID\",
    \"amount\": 30
}"

# Тест 6: Проверка баланса после списания
print_step "TEST 6" "Проверка баланса после списания"
make_request "GET" "/v1/points/$USER_ID/balance"

# Тест 7: Проверка сгорающих баллов
print_step "TEST 7" "Проверка сгорающих баллов в ближайшие 7 дней"
make_request "GET" "/v1/points/$USER_ID/expiring?days=7"

print_step "TEST 8" "Проверка сгорающих баллов в ближайшие 30 дней"
make_request "GET" "/v1/points/$USER_ID/expiring?days=30"

# Тест 9: Резервирование баллов
print_step "TEST 9" "Резервирование 20 баллов"
reserve_response=$(curl -s -X POST "$API_URL/v1/points/reserve" \
    -H "Content-Type: application/json" \
    -d "{\"user_id\": \"$USER_ID\", \"amount\": 20}")

echo "$reserve_response" | jq '.' 2>/dev/null || echo "$reserve_response"
echo ""

if command -v jq &> /dev/null; then
    RESERVATION_ID=$(echo "$reserve_response" | jq -r '.reservation_id')

    if [ "$RESERVATION_ID" != "null" ] && [ -n "$RESERVATION_ID" ]; then
        print_step "INFO" "Reservation ID: $RESERVATION_ID"

        # Тест 10: Проверка баланса после резервирования
        print_step "TEST 10" "Проверка баланса после резервирования"
        make_request "GET" "/v1/points/$USER_ID/balance"

        # Тест 11: Подтверждение резервирования
        print_step "TEST 11" "Подтверждение резервирования"
        make_request "POST" "/v1/points/commit" "{
            \"reservation_id\": \"$RESERVATION_ID\"
        }"

        # Тест 12: Финальный баланс
        print_step "TEST 12" "Финальный баланс"
        make_request "GET" "/v1/points/$USER_ID/balance"
    else
        print_error "Не удалось получить reservation_id"
    fi
else
    echo -e "${YELLOW}jq не установлен, пропускаем тесты с резервированием${NC}\n"
fi

# Тест 13: Попытка списать больше чем есть
print_step "TEST 13" "Попытка списать больше баллов чем есть (ожидается ошибка)"
make_request "POST" "/v1/points/withdraw" "{
    \"user_id\": \"$USER_ID\",
    \"amount\": 9999
}"

echo -e "${BLUE}=== Тестирование завершено ===${NC}\n"
echo -e "${GREEN}✓ Все тесты выполнены${NC}"
echo -e "\nДля просмотра данных в БД:"
echo -e "${YELLOW}psql -U itmo_ledger -d itmo_ledger -c \"SELECT * FROM transactions WHERE user_id = '$USER_ID';\"${NC}"
