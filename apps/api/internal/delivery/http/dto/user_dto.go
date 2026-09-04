package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateUserRequest is the body for POST /v1/users. The account comes from the tenant
// context, never the body, and password is write-only.
type CreateUserRequest struct {
	Name      string      `json:"name" binding:"required,min=1,max=255"`
	Email     string      `json:"email" binding:"required,email,max=255"`
	Password  string      `json:"password" binding:"required,max=72"`
	Role      string      `json:"role" binding:"required,oneof=ADMIN SELLER"`
	BranchIDs []uuid.UUID `json:"branch_ids" binding:"omitempty,dive,required"`
}

// UpdateUserRequest is the body for PUT /v1/users/:userId. It replaces the profile, role and
// branch assignments; is_active omitted leaves the flag alone.
type UpdateUserRequest struct {
	Name      string      `json:"name" binding:"required,min=1,max=255"`
	Email     string      `json:"email" binding:"required,email,max=255"`
	Role      string      `json:"role" binding:"required,oneof=ADMIN SELLER"`
	BranchIDs []uuid.UUID `json:"branch_ids" binding:"omitempty,dive,required"`
	IsActive  *bool       `json:"is_active"`
}

// UserResponse is returned by list, get, create, and update. It never carries the password
// hash.
type UserResponse struct {
	ID          uuid.UUID   `json:"id"`
	Name        string      `json:"name"`
	Email       string      `json:"email"`
	Role        string      `json:"role"`
	IsActive    bool        `json:"is_active"`
	BranchIDs   []uuid.UUID `json:"branch_ids"`
	LastLoginAt *time.Time  `json:"last_login_at"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// UserListResponse is returned by GET /v1/users.
type UserListResponse struct {
	Items []UserResponse `json:"items"`
}
