package data

import (
	"database/sql"
	"github.com/google/uuid"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func setupTestDB(t *testing.T) *sql.DB {
	dsn := "postgres://itmo_ledger:Secret123@localhost/itmo_ledger_test?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skip("Skipping test: database not available")
	}

	err = db.Ping()
	if err != nil {
		t.Skip("Skipping test: database not available")
	}

	// Очистка таблицы перед тестом
	_, err = db.Exec("TRUNCATE TABLE transactions")
	if err != nil {
		t.Fatalf("Failed to truncate table: %v", err)
	}

	return db
}

func TestAddPoints(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := TransactionModel{DB: db}
	userId := uuid.New()

	// Тест добавления баллов без TTL
	transaction, err := model.AddPoints(userId, 100, 0)
	if err != nil {
		t.Fatalf("AddPoints failed: %v", err)
	}

	if transaction.Amount != 100 {
		t.Errorf("Expected amount 100, got %d", transaction.Amount)
	}

	if transaction.RemainingAmount != 100 {
		t.Errorf("Expected remaining amount 100, got %d", transaction.RemainingAmount)
	}

	if transaction.ExpiresAt != nil {
		t.Errorf("Expected no expiration, got %v", transaction.ExpiresAt)
	}

	// Тест добавления баллов с TTL
	transaction2, err := model.AddPoints(userId, 50, 7)
	if err != nil {
		t.Fatalf("AddPoints with TTL failed: %v", err)
	}

	if transaction2.ExpiresAt == nil {
		t.Error("Expected expiration date, got nil")
	}
}

func TestWithdrawPoints(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := TransactionModel{DB: db}
	userId := uuid.New()

	// Добавляем баллы
	_, err := model.AddPoints(userId, 100, 0)
	if err != nil {
		t.Fatalf("AddPoints failed: %v", err)
	}

	// Списываем баллы
	err = model.WithdrawPoints(userId, 50)
	if err != nil {
		t.Fatalf("WithdrawPoints failed: %v", err)
	}

	// Проверяем баланс
	balance, err := model.GetBalance(userId)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if balance.TotalAmount != 50 {
		t.Errorf("Expected balance 50, got %d", balance.TotalAmount)
	}

	// Пытаемся списать больше чем есть
	err = model.WithdrawPoints(userId, 100)
	if err == nil {
		t.Error("Expected insufficient funds error, got nil")
	}
}

func TestFIFOWithdrawal(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := TransactionModel{DB: db}
	userId := uuid.New()

	// Добавляем баллы в разное время
	_, err := model.AddPoints(userId, 50, 0)
	if err != nil {
		t.Fatalf("AddPoints failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	_, err = model.AddPoints(userId, 100, 0)
	if err != nil {
		t.Fatalf("AddPoints failed: %v", err)
	}

	// Списываем 75 баллов
	err = model.WithdrawPoints(userId, 75)
	if err != nil {
		t.Fatalf("WithdrawPoints failed: %v", err)
	}

	// Проверяем что списание произошло по FIFO
	// Должно остаться 75 баллов (50 списано из первой транзакции + 25 из второй)
	balance, err := model.GetBalance(userId)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if balance.TotalAmount != 75 {
		t.Errorf("Expected balance 75, got %d", balance.TotalAmount)
	}
}

func TestGetExpiringPoints(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := TransactionModel{DB: db}
	userId := uuid.New()

	// Добавляем баллы с разными TTL
	_, err := model.AddPoints(userId, 100, 3)
	if err != nil {
		t.Fatalf("AddPoints failed: %v", err)
	}

	_, err = model.AddPoints(userId, 50, 10)
	if err != nil {
		t.Fatalf("AddPoints failed: %v", err)
	}

	// Получаем сгорающие баллы в ближайшие 7 дней
	expiringPoints, err := model.GetExpiringPoints(userId, 7)
	if err != nil {
		t.Fatalf("GetExpiringPoints failed: %v", err)
	}

	// Должны получить одну запись (баллы со сроком 3 дня)
	if len(expiringPoints) != 1 {
		t.Errorf("Expected 1 expiring point entry, got %d", len(expiringPoints))
	}

	if len(expiringPoints) > 0 && expiringPoints[0].Amount != 100 {
		t.Errorf("Expected 100 expiring points, got %d", expiringPoints[0].Amount)
	}
}

func TestReserveAndCommit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := TransactionModel{DB: db}
	userId := uuid.New()

	// Добавляем баллы
	_, err := model.AddPoints(userId, 100, 0)
	if err != nil {
		t.Fatalf("AddPoints failed: %v", err)
	}

	// Резервируем баллы
	reservationId, err := model.ReservePoints(userId, 50)
	if err != nil {
		t.Fatalf("ReservePoints failed: %v", err)
	}

	// Проверяем баланс (должен учитывать зарезервированные баллы)
	balance, err := model.GetBalance(userId)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if balance.TotalAmount != 50 {
		t.Errorf("Expected balance 50 after reservation, got %d", balance.TotalAmount)
	}

	// Подтверждаем резервирование
	err = model.CommitReservation(*reservationId)
	if err != nil {
		t.Fatalf("CommitReservation failed: %v", err)
	}

	// Проверяем баланс после подтверждения
	balance, err = model.GetBalance(userId)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if balance.TotalAmount != 50 {
		t.Errorf("Expected balance 50 after commit, got %d", balance.TotalAmount)
	}
}

func TestReserveAndRollback(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := TransactionModel{DB: db}
	userId := uuid.New()

	// Добавляем баллы
	_, err := model.AddPoints(userId, 100, 0)
	if err != nil {
		t.Fatalf("AddPoints failed: %v", err)
	}

	// Резервируем баллы
	reservationId, err := model.ReservePoints(userId, 50)
	if err != nil {
		t.Fatalf("ReservePoints failed: %v", err)
	}

	// Отменяем резервирование
	err = model.RollbackReservation(*reservationId)
	if err != nil {
		t.Fatalf("RollbackReservation failed: %v", err)
	}

	// Проверяем баланс (баллы должны вернуться)
	balance, err := model.GetBalance(userId)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if balance.TotalAmount != 100 {
		t.Errorf("Expected balance 100 after rollback, got %d", balance.TotalAmount)
	}
}

func TestConcurrentWithdrawals(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model := TransactionModel{DB: db}
	userId := uuid.New()

	// Добавляем баллы
	_, err := model.AddPoints(userId, 100, 0)
	if err != nil {
		t.Fatalf("AddPoints failed: %v", err)
	}

	// Пытаемся списать баллы конкурентно
	done := make(chan error, 2)

	go func() {
		done <- model.WithdrawPoints(userId, 60)
	}()

	go func() {
		done <- model.WithdrawPoints(userId, 60)
	}()

	err1 := <-done
	err2 := <-done

	// Одна из операций должна успешно завершиться, другая - получить ошибку
	if err1 == nil && err2 == nil {
		t.Error("Both concurrent withdrawals succeeded, expected one to fail")
	}

	if err1 != nil && err2 != nil {
		t.Error("Both concurrent withdrawals failed, expected one to succeed")
	}

	// Проверяем что баланс корректный
	balance, err := model.GetBalance(userId)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if balance.TotalAmount != 40 {
		t.Errorf("Expected balance 40, got %d", balance.TotalAmount)
	}
}
