package server

import (
	"encoding/json"
	"net/http"
)

func (s Server) writeJsonResponse(w http.ResponseWriter, response any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.Logger.Errorf("Error encoding response: %+v, err: %v", response, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
