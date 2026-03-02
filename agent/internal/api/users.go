package api

import (
	"net/http"
	"strings"

	"agent/internal/storage"
)

// GET /api/users
func handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	users, err := storage.GetAllUsers()
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if users == nil {
		users = []storage.UserRecord{}
	}
	ok(w, users)
}

// GET /api/users/{id}
func handleUsersWithID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if id == "" {
		apiErr(w, http.StatusBadRequest, "missing user id")
		return
	}
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	u, err := storage.GetUserByID(id)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if u == nil {
		apiErr(w, http.StatusNotFound, "user not found")
		return
	}
	ok(w, u)
}
