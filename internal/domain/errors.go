package domain

import "errors"

var (
	ErrValidation              = errors.New("validation failed")
	ErrUnauthorized            = errors.New("unauthorized")
	ErrForbidden               = errors.New("forbidden")
	ErrNotFound                = errors.New("not found")
	ErrConflict                = errors.New("conflict")
	ErrInvalidCredentials      = errors.New("invalid credentials")
	ErrEmailAlreadyUsed        = errors.New("email already used")
	ErrTokenExpired            = errors.New("token expired")
	ErrInvitationEmailMismatch = errors.New("invitation email mismatch")
	ErrLastOwnerRemoval        = errors.New("cannot remove last owner")
)
