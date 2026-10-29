// Package auth keeps the local accounts and the sessions that sign them in.
//
// The app runs on one machine, so this is deliberately small: email and
// password accounts stored in SQLite, bcrypt hashes, and bearer tokens whose
// hashes are what the database holds. There is no email verification or
// password reset, because there is no mail server to send one from.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// SessionTTL is how long a sign-in lasts before the token stops being accepted.
const SessionTTL = 30 * 24 * time.Hour

const (
	minPasswordLength = 8
	maxPasswordLength = 72 // bcrypt ignores anything after 72 bytes
	maxNameLength     = 80
)

var (
	// ErrEmailTaken means an account with that email already exists.
	ErrEmailTaken = errors.New("an account with that email already exists")
	// ErrInvalidCredentials is returned for both an unknown email and a wrong
	// password, so the form does not reveal which one was wrong.
	ErrInvalidCredentials = errors.New("that email and password do not match")
	// ErrUnauthorized means the bearer token is missing, expired or revoked.
	ErrUnauthorized = errors.New("your session has expired; log in again")
	// ErrNoOwner means nobody has an account on this machine yet.
	ErrNoOwner = errors.New("no one has an account on this machine yet")
)

// ValidationError is a problem with what the person typed, safe to show as-is.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// User is the public shape of an account: no hash, no internals.
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Session is what a successful sign-up or sign-in hands back: the raw token,
// which the browser stores, and the account it belongs to.
type Session struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// Service reads and writes accounts and sessions.
type Service struct {
	store *store.Store
}

// New returns an auth service backed by st.
func New(st *store.Store) *Service {
	return &Service{store: st}
}

// SignUp creates an account and signs it in.
func (s *Service) SignUp(ctx context.Context, name, email, password string) (Session, error) {
	user, err := s.createUser(ctx, name, email, password)
	if err != nil {
		return Session{}, err
	}
	return s.newSession(ctx, user)
}

// SignIn checks a password and starts a session.
func (s *Service) SignIn(ctx context.Context, email, password string) (Session, error) {
	user, err := s.store.UserByEmail(ctx, normalizeEmail(email))
	if errors.Is(err, store.ErrNotFound) {
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return Session{}, ErrInvalidCredentials
	}
	return s.newSession(ctx, user)
}

// OwnerEmail is the address of the account the host creates for a person who
// has never signed up anywhere, such as someone who only ever uses the terminal.
const OwnerEmail = "owner@calabrass.local"

// SignInAsOwner starts a session for the machine's owner: the first account
// ever made here. It is for callers that have already proved they are the
// machine's owner another way (the host secret in the data folder), so there is
// no password to check.
//
// When no account exists yet and create is true, the owner account is made, with
// a random password nobody needs to know. Someone who later signs up in the
// desktop app gets a second account; the owner account stays what the terminal
// uses until they choose otherwise.
func (s *Service) SignInAsOwner(ctx context.Context, create bool) (Session, bool, error) {
	user, err := s.store.OwnerUser(ctx)
	if err == nil {
		session, err := s.newSession(ctx, user)
		return session, false, err
	}
	if !errors.Is(err, store.ErrNotFound) {
		return Session{}, false, err
	}
	if !create {
		return Session{}, false, ErrNoOwner
	}

	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, false, fmt.Errorf("auth: owner password: %w", err)
	}
	created, err := s.createUser(ctx, "Owner", OwnerEmail, hex.EncodeToString(raw))
	if err != nil {
		return Session{}, false, err
	}
	session, err := s.newSession(ctx, created)
	return session, true, err
}

// UserForToken resolves a bearer token, or returns ErrUnauthorized.
func (s *Service) UserForToken(ctx context.Context, token string) (User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, ErrUnauthorized
	}
	user, err := s.store.UserBySession(ctx, hashToken(token), time.Now().UnixMilli())
	if errors.Is(err, store.ErrNotFound) {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, err
	}
	return publicUser(user), nil
}

// SignOut revokes a token. An unknown token is already signed out.
func (s *Service) SignOut(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, hashToken(token))
}

func (s *Service) createUser(ctx context.Context, name, email, password string) (store.UserRecord, error) {
	name = strings.TrimSpace(name)
	email = normalizeEmail(email)

	if name == "" {
		return store.UserRecord{}, &ValidationError{Message: "Enter your name."}
	}
	if len([]rune(name)) > maxNameLength {
		return store.UserRecord{}, &ValidationError{Message: fmt.Sprintf("Keep your name under %d characters.", maxNameLength)}
	}
	if !validEmail(email) {
		return store.UserRecord{}, &ValidationError{Message: "Enter a valid email address."}
	}
	if len(password) < minPasswordLength {
		return store.UserRecord{}, &ValidationError{Message: fmt.Sprintf("Use at least %d characters for your password.", minPasswordLength)}
	}
	if len(password) > maxPasswordLength {
		return store.UserRecord{}, &ValidationError{Message: fmt.Sprintf("Keep your password under %d characters.", maxPasswordLength)}
	}

	if _, err := s.store.UserByEmail(ctx, email); err == nil {
		return store.UserRecord{}, ErrEmailTaken
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.UserRecord{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return store.UserRecord{}, fmt.Errorf("auth: hash password: %w", err)
	}

	user, err := s.store.CreateUser(ctx, store.UserRecord{
		ID:           store.NewID(),
		Name:         name,
		Email:        email,
		PasswordHash: string(hash),
	})
	if err != nil {
		return store.UserRecord{}, err
	}
	return user, nil
}

func (s *Service) newSession(ctx context.Context, user store.UserRecord) (Session, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, fmt.Errorf("auth: create token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expiresAt := time.Now().Add(SessionTTL)

	if err := s.store.CreateSession(ctx, hashToken(token), user.ID, expiresAt.UnixMilli()); err != nil {
		return Session{}, err
	}
	// Opportunistic cleanup; failing to prune old rows must not fail a sign-in.
	_ = s.store.DeleteExpiredSessions(ctx, time.Now().UnixMilli())

	return Session{Token: token, User: publicUser(user)}, nil
}

func publicUser(user store.UserRecord) User {
	return User{ID: user.ID, Name: user.Name, Email: user.Email}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validEmail(email string) bool {
	if email == "" || !strings.Contains(email, "@") {
		return false
	}
	parsed, err := mail.ParseAddress(email)
	return err == nil && strings.EqualFold(parsed.Address, email)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
