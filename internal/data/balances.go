package data

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"time"
)

type Balance struct {
	Id        uuid.UUID `json:"id"`
	Amount    int       `json:"amount"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BalanceModel struct {
	DB *sql.DB
}

func (m BalanceModel) Insert(balance *Balance) error {
	query := `
		INSERT INTO balances (id, amount)
		VALUES ($1, $2)
		RETURNING id, updated_at, amount`
	args := []any{balance.Id, balance.Amount}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.DB.QueryRowContext(ctx, query, args...).Scan(&balance.Id, &balance.UpdatedAt, &balance.Amount)
}

func (m BalanceModel) Get(id uuid.UUID) (*Balance, error) {
	balance := new(Balance)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `
		SELECT id, updated_at, amount
		FROM balances
		WHERE id = $1`
	err := m.DB.QueryRowContext(ctx, query, id).Scan(
		&balance.Id,
		&balance.UpdatedAt,
		&balance.Amount,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return balance, nil
}

func (m BalanceModel) Update(balance *Balance) error {
	query := `
		UPDATE balances
		SET amount = $2, updated_at = $3
		WHERE id = $1
		RETURNING updated_at`
	args := []any{
		balance.Id,
		balance.Amount,
		time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&balance.UpdatedAt)
	if err != nil {
		return err
	}
	return nil
}
