package main

import (
	"encoding/json"
	"net/http"
)

func getMemories(w http.ResponseWriter, r *http.Request) {
	var sessionID string

	if authEnabled {
		sessionCookie, err := r.Cookie("session_id")
		if err != nil {
			if authEnabled {
				http.Error(w, `{"error": true, "message": "Authentication required"}`, http.StatusUnauthorized)
				return
			}
			sessionID = "default"
		} else {
			sessionID = sessionCookie.Value
		}
	} else {
		sessionID = "default"
	}

	memories, err := GetMemories(db, sessionID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, memories)
}

func setMemory(w http.ResponseWriter, r *http.Request) {
	var sessionID string

	if authEnabled {
		sessionCookie, err := r.Cookie("session_id")
		if err != nil {
			if authEnabled {
				http.Error(w, `{"error": true, "message": "Authentication required"}`, http.StatusUnauthorized)
				return
			}
			sessionID = "default"
		} else {
			sessionID = sessionCookie.Value
		}
	} else {
		sessionID = "default"
	}

	var req struct {
		Key        string `json:"key"`
		Value      string `json:"value"`
		Category   string `json:"category"`
		Confidence int    `json:"confidence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if req.Key == "" || req.Value == "" {
		WriteError(w, http.StatusBadRequest, "Key and value required")
		return
	}

	if req.Category == "" {
		req.Category = "preference"
	}
	if req.Confidence == 0 {
		req.Confidence = 80
	}

	if err := SetMemory(db, sessionID, req.Key, req.Value, req.Category, req.Confidence); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, map[string]string{
		"message": "Memory stored successfully",
		"key":     req.Key,
	})
}

func deleteMemory(w http.ResponseWriter, r *http.Request) {
	var sessionID string

	if authEnabled {
		sessionCookie, err := r.Cookie("session_id")
		if err != nil {
			if authEnabled {
				http.Error(w, `{"error": true, "message": "Authentication required"}`, http.StatusUnauthorized)
				return
			}
			sessionID = "default"
		} else {
			sessionID = sessionCookie.Value
		}
	} else {
		sessionID = "default"
	}

	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if req.Key == "" {
		WriteError(w, http.StatusBadRequest, "Key is required")
		return
	}

	if err := DeleteMemory(db, sessionID, req.Key); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, map[string]string{
		"message": "Memory deleted successfully",
		"key":     req.Key,
	})
}

func getSessionIDFromRequest(r *http.Request) string {
	if authEnabled {
		sessionCookie, err := r.Cookie("session_id")
		if err != nil {
			return "default"
		}
		return sessionCookie.Value
	}
	return "default"
}

func searchMemories(w http.ResponseWriter, r *http.Request) {
	var sessionID string

	if authEnabled {
		sessionCookie, err := r.Cookie("session_id")
		if err != nil {
			if authEnabled {
				http.Error(w, `{"error": true, "message": "Authentication required"}`, http.StatusUnauthorized)
				return
			}
			sessionID = "default"
		} else {
			sessionID = sessionCookie.Value
		}
	} else {
		sessionID = "default"
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		WriteError(w, http.StatusBadRequest, "Query parameter 'q' is required")
		return
	}

	memories, err := SearchMemories(db, sessionID, query)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, memories)
}
