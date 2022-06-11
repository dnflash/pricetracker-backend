package server

import (
	"encoding/json"
	"net/http"
)

func (s Server) writeJsonResponse(w http.ResponseWriter, response any, statusCode int) {
	if resp, err := json.Marshal(response); err != nil {
		s.Logger.Errorf("Error encoding response: %+v, err: %v", response, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	} else {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(statusCode)
		if _, err = w.Write(resp); err != nil {
			s.Logger.Errorf("Error writing JSON response: %s, err: %v", resp, err)
		}
	}
}
