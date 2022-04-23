package server

import (
	"github.com/gorilla/mux"
	"net/http"
)

func (s Server) Router() *mux.Router {
	r := mux.NewRouter()

	r.HandleFunc("/api/item/add", s.itemAdd()).Methods(http.MethodPost)
	r.HandleFunc("/api/item/check", s.itemCheck()).Methods(http.MethodPost)
	r.HandleFunc("/api/item/get/{itemID}", s.itemGetOne()).Methods(http.MethodGet)
	r.HandleFunc("/api/item/get", s.itemGetAll()).Methods(http.MethodGet)
	r.HandleFunc("/api/item/history/{itemID}", s.itemHistory()).Methods(http.MethodPost)

	return r
}
