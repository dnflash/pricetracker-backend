package server

import (
	"github.com/gorilla/mux"
	"net/http"
)

func (s Server) Router() *mux.Router {
	r := mux.NewRouter()
	r.Use(s.maxBytesMw)
	r.Use(s.loggingMw)

	api := r.PathPrefix("/api").Subrouter()

	api.HandleFunc("/user/register", s.userRegister()).Methods(http.MethodPost)
	api.HandleFunc("/user/login", s.userLogin()).Methods(http.MethodPost)

	userAPI := api.PathPrefix("/user").Subrouter()
	userAPI.Use(s.authMw)
	userAPI.HandleFunc("/logout", s.userLogout()).Methods(http.MethodPost)
	userAPI.HandleFunc("/info", s.userInfo()).Methods(http.MethodPost)
	userAPI.PathPrefix("").Handler(s.notFoundHandler())

	itemAPI := api.PathPrefix("/item").Subrouter()
	itemAPI.Use(s.authMw)
	itemAPI.HandleFunc("/add", s.itemAdd()).Methods(http.MethodPost)
	itemAPI.HandleFunc("/update", s.itemUpdate()).Methods(http.MethodPost)
	itemAPI.HandleFunc("/remove", s.itemRemove()).Methods(http.MethodPost)
	itemAPI.HandleFunc("/check", s.itemCheck()).Methods(http.MethodPost)
	itemAPI.HandleFunc("/search", s.itemSearch()).Methods(http.MethodGet)
	itemAPI.HandleFunc("/get/{itemID}", s.itemGetOne()).Methods(http.MethodGet)
	itemAPI.HandleFunc("/get", s.itemGetAll()).Methods(http.MethodGet)
	itemAPI.HandleFunc("/history/{itemID}", s.itemHistory()).Methods(http.MethodPost)
	itemAPI.PathPrefix("").Handler(s.notFoundHandler())

	r.PathPrefix("").Handler(s.notFoundHandler())

	return r
}
