package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"`
}

var (
	sessions    = make(map[string]Session)
	sessionMu   sync.RWMutex
	sessionTTL  = 24 * time.Hour
	sessionKey  string
	authEnabled = false
	adminUser   User
)

func init() {
	sessionKey = generateSecureToken(32)
}

func generateSecureToken(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password + sessionKey))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func InitAuth(username, password string) {
	if username == "" || password == "" {
		authEnabled = false
		return
	}
	authEnabled = true

	adminUser = User{
		ID:       "admin",
		Username: username,
		Password: hashPassword(password),
	}
}

func IsAuthEnabled() bool {
	return authEnabled
}

func CreateSession(userID string) string {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	sessionID := generateSecureToken(32)
	sessions[sessionID] = Session{
		ID:        sessionID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(sessionTTL),
	}
	return sessionID
}

func ValidateSession(sessionID string) bool {
	if sessionID == "" {
		return false
	}

	sessionMu.RLock()
	defer sessionMu.RUnlock()

	session, exists := sessions[sessionID]
	if !exists {
		return false
	}

	if time.Now().After(session.ExpiresAt) {
		delete(sessions, sessionID)
		return false
	}

	return true
}

func DestroySession(sessionID string) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	delete(sessions, sessionID)
}

func CleanupSessions() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		sessionMu.Lock()
		now := time.Now()
		for id, session := range sessions {
			if now.After(session.ExpiresAt) {
				delete(sessions, id)
			}
		}
		sessionMu.Unlock()
	}
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authEnabled {
			next.ServeHTTP(w, r)
			return
		}

		sessionID, err := r.Cookie("session_id")
		if err != nil {
			http.Error(w, `{"error": true, "message": "Authentication required"}`, http.StatusUnauthorized)
			return
		}

		if !ValidateSession(sessionID.Value) {
			http.Error(w, `{"error": true, "message": "Invalid or expired session"}`, http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func OptionalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authEnabled {
			next.ServeHTTP(w, r)
			return
		}

		sessionID, err := r.Cookie("session_id")
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		if !ValidateSession(sessionID.Value) {
			next.ServeHTTP(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if !authEnabled {
		WriteJSON(w, map[string]interface{}{
			"status":  "disabled",
			"message": "Authentication is not configured",
		})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if req.Username != adminUser.Username {
		WriteError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	if hashPassword(req.Password) != adminUser.Password {
		WriteError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	sessionID := CreateSession(adminUser.ID)

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(sessionTTL),
	})

	WriteJSON(w, map[string]interface{}{
		"status":     "success",
		"session_id": sessionID,
		"user": map[string]string{
			"id":       adminUser.ID,
			"username": adminUser.Username,
		},
	})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	sessionID, err := r.Cookie("session_id")
	if err == nil {
		DestroySession(sessionID.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	WriteJSON(w, map[string]string{"status": "logged_out"})
}

func sessionStatusHandler(w http.ResponseWriter, r *http.Request) {
	if !authEnabled {
		WriteJSON(w, map[string]interface{}{
			"enabled":       false,
			"authenticated": false,
		})
		return
	}

	sessionID, err := r.Cookie("session_id")
	if err != nil {
		WriteJSON(w, map[string]interface{}{
			"enabled":       true,
			"authenticated": false,
		})
		return
	}

	authenticated := ValidateSession(sessionID.Value)

	WriteJSON(w, map[string]interface{}{
		"enabled":       true,
		"authenticated": authenticated,
		"user": map[string]string{
			"id":       adminUser.ID,
			"username": adminUser.Username,
		},
	})
}

func adminHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Admin - OllamaGoWeb</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, sans-serif; background: #f5f5f5; padding: 20px; }
        .container { max-width: 400px; margin: 50px auto; background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { margin-bottom: 20px; color: #333; }
        .form-group { margin-bottom: 15px; }
        label { display: block; margin-bottom: 5px; color: #666; }
        input { width: 100%; padding: 10px; border: 1px solid #ddd; border-radius: 4px; font-size: 14px; }
        button { width: 100%; padding: 10px; background: #4f39f6; color: white; border: none; border-radius: 4px; cursor: pointer; }
        button:hover { background: #3b2fd6; }
        .error { color: #ef4444; margin-bottom: 15px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Admin Login</h1>
        <div id="error" class="error"></div>
        <form id="loginForm">
            <div class="form-group">
                <label for="username">Username</label>
                <input type="text" id="username" name="username" required>
            </div>
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required>
            </div>
            <button type="submit">Login</button>
        </form>
    </div>
    <script>
        document.getElementById('loginForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            const username = document.getElementById('username').value;
            const password = document.getElementById('password').value;
            try {
                const res = await fetch('/api/auth/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username, password })
                });
                const data = await res.json();
                if (res.ok) {
                    window.location.href = '/';
                } else {
                    document.getElementById('error').textContent = data.message || 'Login failed';
                }
            } catch (err) {
                document.getElementById('error').textContent = 'Connection error';
            }
        });
    </script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
		return
	}
}
