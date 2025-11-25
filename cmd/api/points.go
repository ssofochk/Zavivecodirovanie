package main

import (
	"errors"
	"github.com/google/uuid"
	"net/http"
	"simple-ledger.itmo.ru/internal/validator"
	"strconv"
)

type addPointsRequest struct {
	UserId  string `json:"user_id"`
	Amount  int    `json:"amount"`
	TtlDays int    `json:"ttl_days"`
}

type withdrawPointsRequest struct {
	UserId string `json:"user_id"`
	Amount int    `json:"amount"`
}

type reservePointsRequest struct {
	UserId string `json:"user_id"`
	Amount int    `json:"amount"`
}

type commitReservationRequest struct {
	ReservationId string `json:"reservation_id"`
}

type rollbackReservationRequest struct {
	ReservationId string `json:"reservation_id"`
}

// addPointsHandler обрабатывает добавление бонусных баллов
func (app *application) addPointsHandler(w http.ResponseWriter, r *http.Request) {
	var req addPointsRequest
	err := app.readJSON(w, r, &req)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	userId, err := uuid.Parse(req.UserId)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid user_id"))
		return
	}

	v := validator.New()
	v.Check(req.Amount > 0, "amount", "must be positive")
	v.Check(req.TtlDays >= 0, "ttl_days", "must be non-negative")

	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	transaction, err := app.models.Transactions.AddPoints(userId, req.Amount, req.TtlDays)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusCreated, transaction, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// withdrawPointsHandler обрабатывает списание бонусных баллов
func (app *application) withdrawPointsHandler(w http.ResponseWriter, r *http.Request) {
	var req withdrawPointsRequest
	err := app.readJSON(w, r, &req)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	userId, err := uuid.Parse(req.UserId)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid user_id"))
		return
	}

	v := validator.New()
	v.Check(req.Amount > 0, "amount", "must be positive")

	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.models.Transactions.WithdrawPoints(userId, req.Amount)
	if err != nil {
		if err.Error() == "insufficient funds" {
			app.badRequestResponse(w, r, err)
		} else {
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	response := map[string]string{"message": "points withdrawn successfully"}
	err = app.writeJSON(w, http.StatusOK, response, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// getBalanceHandler возвращает текущий баланс пользователя
func (app *application) getBalanceHandler(w http.ResponseWriter, r *http.Request) {
	userId, err := app.readIDParam(r)
	if err != nil || userId == uuid.Nil {
		app.notFoundResponse(w, r)
		return
	}

	balance, err := app.models.Transactions.GetBalance(userId)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, balance, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// getExpiringPointsHandler возвращает информацию о сгорающих баллах
func (app *application) getExpiringPointsHandler(w http.ResponseWriter, r *http.Request) {
	userId, err := app.readIDParam(r)
	if err != nil || userId == uuid.Nil {
		app.notFoundResponse(w, r)
		return
	}

	daysStr := r.URL.Query().Get("days")
	days := 7 // по умолчанию 7 дней

	if daysStr != "" {
		parsedDays, err := strconv.Atoi(daysStr)
		if err != nil || parsedDays <= 0 {
			app.badRequestResponse(w, r, errors.New("invalid days parameter"))
			return
		}
		days = parsedDays
	}

	expiringPoints, err := app.models.Transactions.GetExpiringPoints(userId, days)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	response := map[string]interface{}{
		"user_id":         userId,
		"days":            days,
		"expiring_points": expiringPoints,
	}

	err = app.writeJSON(w, http.StatusOK, response, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// reservePointsHandler резервирует баллы для последующего списания
func (app *application) reservePointsHandler(w http.ResponseWriter, r *http.Request) {
	var req reservePointsRequest
	err := app.readJSON(w, r, &req)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	userId, err := uuid.Parse(req.UserId)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid user_id"))
		return
	}

	v := validator.New()
	v.Check(req.Amount > 0, "amount", "must be positive")

	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	reservationId, err := app.models.Transactions.ReservePoints(userId, req.Amount)
	if err != nil {
		if err.Error() == "insufficient funds" {
			app.badRequestResponse(w, r, err)
		} else {
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	response := map[string]string{
		"reservation_id": reservationId.String(),
		"message":        "points reserved successfully",
	}

	err = app.writeJSON(w, http.StatusCreated, response, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// commitReservationHandler подтверждает резервирование
func (app *application) commitReservationHandler(w http.ResponseWriter, r *http.Request) {
	var req commitReservationRequest
	err := app.readJSON(w, r, &req)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	reservationId, err := uuid.Parse(req.ReservationId)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid reservation_id"))
		return
	}

	err = app.models.Transactions.CommitReservation(reservationId)
	if err != nil {
		if err.Error() == "reservation not found or already processed" {
			app.notFoundResponse(w, r)
		} else {
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	response := map[string]string{"message": "reservation committed successfully"}
	err = app.writeJSON(w, http.StatusOK, response, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// rollbackReservationHandler отменяет резервирование
func (app *application) rollbackReservationHandler(w http.ResponseWriter, r *http.Request) {
	var req rollbackReservationRequest
	err := app.readJSON(w, r, &req)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	reservationId, err := uuid.Parse(req.ReservationId)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid reservation_id"))
		return
	}

	err = app.models.Transactions.RollbackReservation(reservationId)
	if err != nil {
		if err.Error() == "reservation not found or already processed" {
			app.notFoundResponse(w, r)
		} else {
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	response := map[string]string{"message": "reservation rolled back successfully"}
	err = app.writeJSON(w, http.StatusOK, response, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
