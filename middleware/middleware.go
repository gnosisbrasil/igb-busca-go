// Package middleware mirrors SuperGnosis.Api Middleware + CORS policy.
package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"igb-busca-go/model"
	"igb-busca-go/repository"
	"igb-busca-go/service"
)

// userContextKey carries the authenticated user (or nil) like
// HttpContext.Items["User"].
type userContextKey struct{}

// UserFromContext returns the authenticated user, or nil when anonymous
// (missing Authorization header or unknown user id).
func UserFromContext(ctx context.Context) *model.User {
	u, _ := ctx.Value(userContextKey{}).(*model.User)
	return u
}

// PerfilOf returns the caller's perfil or "" when anonymous, mirroring
// user?.Perfil / string.IsNullOrEmpty checks in the controllers.
func PerfilOf(u *model.User) string {
	if u == nil || u.Perfil == nil {
		return ""
	}
	return *u.Perfil
}

// JWT mirrors JwtMiddleware: when an Authorization header is present, its
// last space-separated part is validated as a token and the "id" claim
// (32-bit int) loads the user. Any failure surfaces as HTTP 500 with an
// empty body, exactly like the uncaught exception in the original. A
// missing header means anonymous access (some endpoints allow it).
func JWT(users repository.UserStore, tokens *service.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			values, present := r.Header["Authorization"]
			if present {
				token := ""
				if len(values) > 0 {
					parts := strings.Split(values[0], " ")
					token = parts[len(parts)-1]
				}
				claims, err := tokens.ValidateToken(token)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				idRaw, _ := claims["id"].(string)
				id, err := strconv.ParseInt(idRaw, 10, 32)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				user, err := users.GetUserByID(r.Context(), id)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if user.ID != 0 {
					r = r.WithContext(context.WithValue(r.Context(), userContextKey{}, &user))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS mirrors the "AllowAll" policy: any origin, method and header.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
