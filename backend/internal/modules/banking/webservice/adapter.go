// Package webservice is the bank web-service channel (20 §2 yol 1, K7 seviye 2):
// the taxpayer instructs the branch, the bank allows our fixed IPs and issues
// a service user, the portal reads accounts and movements with it.
//
// Adapters only read (20 §8). Credentials live in the vault (purpose
// "bank_connection"); this package never logs them.
package webservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Reason is the bank-neutral failure code shown to the taxpayer (20 §5.4).
type Reason string

const (
	ReasonIPRejected         Reason = "ip_rejected"
	ReasonUnauthorized       Reason = "unauthorized"
	ReasonBadCredentials     Reason = "bad_credentials"
	ReasonInstructionMissing Reason = "instruction_missing"
	ReasonAccountNotDefined  Reason = "account_not_defined"
	ReasonBankError          Reason = "bank_error"
	ReasonRateLimited        Reason = "rate_limited"
	ReasonInvalidRequest     Reason = "invalid_request"
)

// Message is the Turkish text for a reason (20 §5.4).
func (r Reason) Message() string {
	switch r {
	case ReasonIPRejected:
		return "Bankanız IP tanımını henüz yapmamış. Şubeye IP adreslerinin tanımlanıp tanımlanmadığını sorun."
	case ReasonUnauthorized:
		return "Banka erişime izin vermedi. Şubeye talimatın ve IP tanımının tamamlandığını sorun."
	case ReasonBadCredentials:
		return "Kullanıcı adı veya şifre hatalı. Bankadan gelen bilgileri kontrol edin."
	case ReasonInstructionMissing:
		return "Hesap için bankada talimat bulunamadı. Şubeye talimatın işlenip işlenmediğini sorun."
	case ReasonAccountNotDefined:
		return "Bu hesap servise tanımlı değil. Şubeden hesabın talimata eklenmesini isteyin."
	case ReasonRateLimited:
		return "Bankanın günlük sorgu sınırına ulaşıldı. Yarın otomatik devam edilecek."
	case ReasonInvalidRequest:
		return "Banka isteği geçersiz buldu. Destek ekibine bildirildi."
	default:
		return "Banka geçici olarak yanıt vermiyor. Otomatik tekrar denenecek."
	}
}

// Error is a bank answer mapped to a Reason; Code is the bank's own code.
type Error struct {
	Reason  Reason
	Code    string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("bank %s: %s %s", e.Reason, e.Code, e.Message)
}

// AsError returns the *Error inside err, or a bank_error wrapper for
// transport failures.
func AsError(err error) *Error {
	var be *Error
	if errors.As(err, &be) {
		return be
	}
	return &Error{Reason: ReasonBankError, Message: err.Error()}
}

// Limits drive the window planner (20 §6 BankLimits).
type Limits struct {
	MaxRangeDays   int // 0: no documented limit
	MaxHistoryDays int // 0: unknown
	MinInterval    time.Duration
}

// CredentialField is one input of the "bank credentials" form (20 §5.3).
type CredentialField struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Secret  bool   `json:"secret"`
	Pattern string `json:"pattern,omitempty"`
	Help    string `json:"help,omitempty"`
}

// Credentials are the values of CredentialFields, read from the vault.
type Credentials map[string]string

// Window is one query (Europe/Istanbul wall clock).
type Window struct {
	Start, End time.Time
	// Account narrows the query to one account number; empty: all accounts.
	Account string
}

// Account is a bank account as the service reports it.
type Account struct {
	Number     string                `json:"number"`
	IBAN       string                `json:"iban"`
	Currency   string                `json:"currency"`
	BranchCode string                `json:"branch_code,omitempty"`
	BranchName string                `json:"branch_name,omitempty"`
	Nickname   string                `json:"nickname,omitempty"`
	Kind       string                `json:"kind,omitempty"` // demand | term
	Active     bool                  `json:"active"`
	OwnerTaxID string                `json:"owner_tax_id,omitempty"`
	OwnerName  string                `json:"owner_name,omitempty"`
	Opening    *string               `json:"opening_balance,omitempty"`
	Balance    *string               `json:"balance,omitempty"`
	Available  *string               `json:"available_balance,omitempty"`
	Movements  []banking.Transaction `json:"-"`
}

// Result is one window's answer.
type Result struct {
	Accounts []Account
}

// Adapter is one bank's web service. Fetch runs a single window; splitting a
// range into windows is the caller's job (20 §6).
type Adapter interface {
	BankID() string
	BankName() string
	SourceSystem() string
	Limits() Limits
	CredentialFields() []CredentialField
	Fetch(ctx context.Context, creds Credentials, w Window) (Result, error)
}

// Windows splits [from, to] into consecutive windows of at most days days
// (days <= 0: one window). Each window ends 1 minute before the next starts.
func Windows(from, to time.Time, days int) []Window {
	if !to.After(from) {
		return nil
	}
	if days <= 0 {
		return []Window{{Start: from, End: to}}
	}
	var out []Window
	for s := from; s.Before(to); {
		e := s.AddDate(0, 0, days)
		if e.After(to) {
			e = to
		} else {
			e = e.Add(-time.Minute)
		}
		out = append(out, Window{Start: s, End: e})
		s = e.Add(time.Minute)
	}
	return out
}
