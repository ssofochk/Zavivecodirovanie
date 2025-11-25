package main

import (
	"github.com/julienschmidt/httprouter"
	"net/http"
)

func (app *application) routes() *httprouter.Router {
	router := httprouter.New()

	router.NotFound = http.HandlerFunc(app.notFoundResponse)
	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)

	// Старые endpoints (для обратной совместимости)
	router.HandlerFunc(http.MethodPost, "/v1/transactions", app.createTransactionHandler)
	router.HandlerFunc(http.MethodGet, "/v1/users/:id/balance", app.showUserBalanceHandler)

	// Новые endpoints для работы с бонусными баллами
	router.HandlerFunc(http.MethodPost, "/v1/points/add", app.addPointsHandler)
	router.HandlerFunc(http.MethodPost, "/v1/points/withdraw", app.withdrawPointsHandler)
	router.HandlerFunc(http.MethodGet, "/v1/points/:id/balance", app.getBalanceHandler)
	router.HandlerFunc(http.MethodGet, "/v1/points/:id/expiring", app.getExpiringPointsHandler)

	// Endpoints для резервирования
	router.HandlerFunc(http.MethodPost, "/v1/points/reserve", app.reservePointsHandler)
	router.HandlerFunc(http.MethodPost, "/v1/points/commit", app.commitReservationHandler)
	router.HandlerFunc(http.MethodPost, "/v1/points/rollback", app.rollbackReservationHandler)

	return router
}
