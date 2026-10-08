package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"igb-busca-go/model"
)

// UserStore mirrors IUserRepository.
type UserStore interface {
	GetUsers(ctx context.Context) ([]model.User, error)
	GetUserByID(ctx context.Context, id int64) (model.User, error)
}

// UserRepository mirrors UserRepository.cs.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a UserRepository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// GetUsers mirrors GetUsersAsync: SELECT * FROM gnosis.users.
func (r *UserRepository) GetUsers(ctx context.Context) ([]model.User, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, perfil, senha FROM gnosis.users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []model.User{}
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Name, &u.Perfil, &u.Senha); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// GetUserByID mirrors GetUserByIdAsync (zero User when not found,
// error on real DB failures just like QueryFirstOrDefaultAsync throws).
func (r *UserRepository) GetUserByID(ctx context.Context, id int64) (model.User, error) {
	var u model.User
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, perfil, senha FROM gnosis.users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Name, &u.Perfil, &u.Senha)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, nil
		}
		return model.User{}, err
	}
	return u, nil
}
