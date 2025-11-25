package data

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"time"
)

type Transaction struct {
	Id              uuid.UUID  `json:"id"`
	UserId          uuid.UUID  `json:"user_id"`
	Amount          int        `json:"amount"`
	Type            string     `json:"type"`
	Status          string     `json:"status"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	ReservationId   *uuid.UUID `json:"reservation_id,omitempty"`
	RemainingAmount int        `json:"remaining_amount"`
}

type UserBalance struct {
	UserId         uuid.UUID          `json:"user_id"`
	TotalAmount    int                `json:"total_amount"`
	ExpiringPoints []ExpiringPoints   `json:"expiring_points,omitempty"`
}

type ExpiringPoints struct {
	Amount    int       `json:"amount"`
	ExpiresAt time.Time `json:"expires_at"`
}

type TransactionModel struct {
	DB *sql.DB
}

// AddPoints добавляет бонусные баллы пользователю с указанным TTL в днях
func (m TransactionModel) AddPoints(userId uuid.UUID, amount int, ttlDays int) (*Transaction, error) {
	tx, err := m.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	transaction := &Transaction{
		Id:              uuid.New(),
		UserId:          userId,
		Amount:          amount,
		Type:            "deposit",
		Status:          "completed",
		RemainingAmount: amount,
		CreatedAt:       time.Now(),
	}

	if ttlDays > 0 {
		expiresAt := time.Now().AddDate(0, 0, ttlDays)
		transaction.ExpiresAt = &expiresAt
	}

	query := `
		INSERT INTO transactions (id, user_id, amount, type, status, expires_at, remaining_amount)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`

	err = tx.QueryRow(query,
		transaction.Id,
		transaction.UserId,
		transaction.Amount,
		transaction.Type,
		transaction.Status,
		transaction.ExpiresAt,
		transaction.RemainingAmount,
	).Scan(&transaction.CreatedAt)

	if err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return transaction, nil
}

// WithdrawPoints списывает бонусные баллы по принципу FIFO (сначала самые старые)
func (m TransactionModel) WithdrawPoints(userId uuid.UUID, amount int) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Блокируем строки пользователя для предотвращения race condition
	query := `
		SELECT id, remaining_amount, created_at
		FROM transactions
		WHERE user_id = $1
			AND type = 'deposit'
			AND status = 'completed'
			AND remaining_amount > 0
			AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at ASC
		FOR UPDATE`

	rows, err := tx.Query(query, userId)
	if err != nil {
		return err
	}
	defer rows.Close()

	type depositInfo struct {
		id              uuid.UUID
		remainingAmount int
		createdAt       time.Time
	}

	var deposits []depositInfo
	totalAvailable := 0

	for rows.Next() {
		var d depositInfo
		if err := rows.Scan(&d.id, &d.remainingAmount, &d.createdAt); err != nil {
			return err
		}
		deposits = append(deposits, d)
		totalAvailable += d.remainingAmount
	}

	if err = rows.Err(); err != nil {
		return err
	}

	if totalAvailable < amount {
		return errors.New("insufficient funds")
	}

	// Списываем баллы начиная с самых старых
	remainingToWithdraw := amount
	for _, deposit := range deposits {
		if remainingToWithdraw == 0 {
			break
		}

		toDeduct := deposit.remainingAmount
		if toDeduct > remainingToWithdraw {
			toDeduct = remainingToWithdraw
		}

		updateQuery := `
			UPDATE transactions
			SET remaining_amount = remaining_amount - $1
			WHERE id = $2`

		_, err = tx.Exec(updateQuery, toDeduct, deposit.id)
		if err != nil {
			return err
		}

		remainingToWithdraw -= toDeduct
	}

	// Создаем запись о списании
	withdrawalId := uuid.New()
	insertQuery := `
		INSERT INTO transactions (id, user_id, amount, type, status, remaining_amount)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err = tx.Exec(insertQuery, withdrawalId, userId, amount, "withdrawal", "completed", 0)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	return nil
}

// GetBalance возвращает текущий баланс пользователя
func (m TransactionModel) GetBalance(userId uuid.UUID) (*UserBalance, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `
		SELECT COALESCE(SUM(remaining_amount), 0) as total
		FROM transactions
		WHERE user_id = $1
			AND type = 'deposit'
			AND status = 'completed'
			AND remaining_amount > 0
			AND (expires_at IS NULL OR expires_at > NOW())`

	var totalAmount int
	err := m.DB.QueryRowContext(ctx, query, userId).Scan(&totalAmount)
	if err != nil {
		return nil, err
	}

	return &UserBalance{
		UserId:      userId,
		TotalAmount: totalAmount,
	}, nil
}

// GetExpiringPoints возвращает информацию о сгорающих баллах в ближайшие дни
func (m TransactionModel) GetExpiringPoints(userId uuid.UUID, days int) ([]ExpiringPoints, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `
		SELECT expires_at, SUM(remaining_amount) as amount
		FROM transactions
		WHERE user_id = $1
			AND type = 'deposit'
			AND status = 'completed'
			AND remaining_amount > 0
			AND expires_at IS NOT NULL
			AND expires_at > NOW()
			AND expires_at <= NOW() + INTERVAL '1 day' * $2
		GROUP BY expires_at
		ORDER BY expires_at ASC`

	rows, err := m.DB.QueryContext(ctx, query, userId, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expiringPoints []ExpiringPoints
	for rows.Next() {
		var ep ExpiringPoints
		if err := rows.Scan(&ep.ExpiresAt, &ep.Amount); err != nil {
			return nil, err
		}
		expiringPoints = append(expiringPoints, ep)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return expiringPoints, nil
}

// ReservePoints резервирует баллы для последующего списания
func (m TransactionModel) ReservePoints(userId uuid.UUID, amount int) (*uuid.UUID, error) {
	tx, err := m.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Блокируем строки пользователя
	query := `
		SELECT id, remaining_amount
		FROM transactions
		WHERE user_id = $1
			AND type = 'deposit'
			AND status = 'completed'
			AND remaining_amount > 0
			AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at ASC
		FOR UPDATE`

	rows, err := tx.Query(query, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type depositInfo struct {
		id              uuid.UUID
		remainingAmount int
	}

	var deposits []depositInfo
	totalAvailable := 0

	for rows.Next() {
		var d depositInfo
		if err := rows.Scan(&d.id, &d.remainingAmount); err != nil {
			return nil, err
		}
		deposits = append(deposits, d)
		totalAvailable += d.remainingAmount
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	if totalAvailable < amount {
		return nil, errors.New("insufficient funds")
	}

	reservationId := uuid.New()

	// Резервируем баллы начиная с самых старых
	remainingToReserve := amount
	for _, deposit := range deposits {
		if remainingToReserve == 0 {
			break
		}

		toReserve := deposit.remainingAmount
		if toReserve > remainingToReserve {
			toReserve = remainingToReserve
		}

		// Создаем запись резервирования
		reserveQuery := `
			INSERT INTO transactions (id, user_id, amount, type, status, reservation_id, remaining_amount)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`

		_, err = tx.Exec(reserveQuery, uuid.New(), userId, toReserve, "reserve", "reserved", reservationId, toReserve)
		if err != nil {
			return nil, err
		}

		// Уменьшаем доступное количество в исходной транзакции
		updateQuery := `
			UPDATE transactions
			SET remaining_amount = remaining_amount - $1
			WHERE id = $2`

		_, err = tx.Exec(updateQuery, toReserve, deposit.id)
		if err != nil {
			return nil, err
		}

		remainingToReserve -= toReserve
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return &reservationId, nil
}

// CommitReservation подтверждает резервирование и окончательно списывает баллы
func (m TransactionModel) CommitReservation(reservationId uuid.UUID) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Помечаем резервирование как завершенное
	updateQuery := `
		UPDATE transactions
		SET status = 'completed', type = 'withdrawal'
		WHERE reservation_id = $1 AND status = 'reserved'`

	result, err := tx.Exec(updateQuery, reservationId)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("reservation not found or already processed")
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	return nil
}

// RollbackReservation отменяет резервирование и возвращает баллы
func (m TransactionModel) RollbackReservation(reservationId uuid.UUID) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Получаем все резервирования
	query := `
		SELECT id, amount
		FROM transactions
		WHERE reservation_id = $1 AND status = 'reserved'
		FOR UPDATE`

	rows, err := tx.Query(query, reservationId)
	if err != nil {
		return err
	}
	defer rows.Close()

	type reserveInfo struct {
		id     uuid.UUID
		amount int
	}

	var reserves []reserveInfo
	for rows.Next() {
		var r reserveInfo
		if err := rows.Scan(&r.id, &r.amount); err != nil {
			return err
		}
		reserves = append(reserves, r)
	}

	if err = rows.Err(); err != nil {
		return err
	}

	if len(reserves) == 0 {
		return errors.New("reservation not found or already processed")
	}

	// Отменяем резервирование
	cancelQuery := `
		UPDATE transactions
		SET status = 'cancelled'
		WHERE reservation_id = $1 AND status = 'reserved'`

	_, err = tx.Exec(cancelQuery, reservationId)
	if err != nil {
		return err
	}

	// Возвращаем баллы обратно в deposit транзакции
	// Находим оригинальные deposit транзакции и возвращаем баллы
	for _, reserve := range reserves {
		// Получаем user_id из резервирования
		var userId uuid.UUID
		err = tx.QueryRow(`SELECT user_id FROM transactions WHERE id = $1`, reserve.id).Scan(&userId)
		if err != nil {
			return err
		}

		// Находим самую раннюю deposit транзакцию с недостающими баллами
		returnQuery := `
			UPDATE transactions t
			SET remaining_amount = remaining_amount + $1
			WHERE id = (
				SELECT id FROM transactions
				WHERE user_id = $2
					AND type = 'deposit'
					AND status = 'completed'
					AND (expires_at IS NULL OR expires_at > NOW())
				ORDER BY created_at ASC
				LIMIT 1
			)`

		_, err = tx.Exec(returnQuery, reserve.amount, userId)
		if err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	return nil
}
