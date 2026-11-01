package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// A device is another machine a person trusts to use this host: their laptop's
// terminal, a phone. It gets there by pairing, not by signing in:
//
//  1. At the machine the host runs on, the owner asks for a pairing code.
//  2. On the other machine they give that code, once, with a name for the device.
//  3. The host answers with a session that belongs to that device.
//
// No password crosses the network, and a code is worth nothing after one use or
// ten minutes. The device's session is tied to the device, so removing the device
// signs it out immediately, from the machine the host runs on.

const (
	// PairingTTL is how long a code can be used for.
	PairingTTL = 10 * time.Minute
	// DeviceSessionTTL is how long a paired device stays paired without
	// re-pairing. Longer than a browser's sign-in: a terminal is not signed in
	// again by hand every month.
	DeviceSessionTTL = 90 * 24 * time.Hour

	// codeAlphabet leaves out the characters people confuse (0 O 1 I L) and
	// vowels that spell words.
	codeAlphabet = "BCDFGHJKMNPQRSTVWXYZ23456789"
	codeLength   = 8
)

// ErrBadPairingCode is every reason a code does not work, on purpose: a caller
// guessing codes learns nothing from which one it hit.
var ErrBadPairingCode = errors.New("that pairing code is not valid, or it has expired")

// Pairing is a fresh code, shown once to the person who asked for it.
type Pairing struct {
	// Code is what to type on the other machine, as "ABCD-EFGH".
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// StartPairing makes a one-time code for a person.
func (s *Service) StartPairing(ctx context.Context, userID string) (Pairing, error) {
	// A random byte modulo 28 would favour some symbols; reject the top of the
	// range so every symbol is equally likely.
	chars := make([]byte, 0, codeLength)
	limit := byte(256 - 256%len(codeAlphabet))
	for len(chars) < codeLength {
		b := make([]byte, 1)
		if _, err := rand.Read(b); err != nil {
			return Pairing{}, fmt.Errorf("auth: pairing code: %w", err)
		}
		if b[0] >= limit {
			continue
		}
		chars = append(chars, codeAlphabet[int(b[0])%len(codeAlphabet)])
	}
	code := string(chars)
	expires := time.Now().Add(PairingTTL)

	if err := s.store.CreatePairingCode(ctx, hashCode(code), userID, expires.UnixMilli()); err != nil {
		return Pairing{}, err
	}
	return Pairing{Code: code[:4] + "-" + code[4:], ExpiresAt: expires}, nil
}

// PairedDevice is a device and the session it was given.
type PairedDevice struct {
	Session Session      `json:"session"`
	Device  store.Device `json:"device"`
}

// ClaimPairing trades a code for a device session. It is the only way in for a
// machine that is not this one.
func (s *Service) ClaimPairing(ctx context.Context, code, deviceName string) (PairedDevice, error) {
	normalized := normalizeCode(code)
	if len(normalized) != codeLength {
		return PairedDevice{}, ErrBadPairingCode
	}
	userID, err := s.store.ConsumePairingCode(ctx, hashCode(normalized), time.Now().UnixMilli())
	if errors.Is(err, store.ErrNotFound) {
		return PairedDevice{}, ErrBadPairingCode
	}
	if err != nil {
		return PairedDevice{}, err
	}

	user, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return PairedDevice{}, err
	}
	device := store.Device{ID: store.NewID(), UserID: userID, Name: cleanDeviceName(deviceName)}
	if err := s.store.CreateDevice(ctx, device); err != nil {
		return PairedDevice{}, err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return PairedDevice{}, fmt.Errorf("auth: create token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := s.store.CreateDeviceSession(ctx, hashToken(token), userID, device.ID,
		time.Now().Add(DeviceSessionTTL).UnixMilli()); err != nil {
		_, _ = s.store.RevokeDevice(ctx, userID, device.ID)
		return PairedDevice{}, err
	}
	return PairedDevice{Session: Session{Token: token, User: publicUser(user)}, Device: device}, nil
}

// Devices lists a person's paired devices.
func (s *Service) Devices(ctx context.Context, userID string) ([]store.Device, error) {
	return s.store.ListDevices(ctx, userID)
}

// ErrNoSuchDevice means the person has no device with that id.
var ErrNoSuchDevice = errors.New("there is no such device")

// RevokeDevice removes a device. Its session stops working at once.
func (s *Service) RevokeDevice(ctx context.Context, userID, deviceID string) error {
	ok, err := s.store.RevokeDevice(ctx, userID, deviceID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoSuchDevice
	}
	return nil
}

// DeviceForToken says whether a token belongs to a paired device, and which. A
// password sign-in made on this machine belongs to none.
func (s *Service) DeviceForToken(ctx context.Context, token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	now := time.Now().UnixMilli()
	id, err := s.store.DeviceForSession(ctx, hashToken(token), now)
	if err != nil {
		return "", false
	}
	return id, true
}

// TouchDevice records that a device was just used.
func (s *Service) TouchDevice(ctx context.Context, deviceID string) {
	s.store.TouchDevice(ctx, deviceID, time.Now().UnixMilli())
}

func hashCode(normalized string) string {
	sum := sha256.Sum256([]byte("calabrass-pairing:" + normalized))
	return hex.EncodeToString(sum[:])
}

// normalizeCode reads what a person typed: any case, with or without the dash or
// spaces.
func normalizeCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(code) {
		if r == '-' || unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// cleanDeviceName makes a name fit to show: no control characters, one line, not
// long. It is typed by the person pairing, but it is shown on screens later.
func cleanDeviceName(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case unicode.IsControl(r), r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
			continue
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if r := []rune(out); len(r) > 60 {
		out = string(r[:60])
	}
	if out == "" {
		return "Unnamed device"
	}
	return out
}
