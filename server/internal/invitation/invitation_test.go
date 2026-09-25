package invitation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/invitation"
)

type recordingRepo struct {
	expiresAt    *time.Time
	tokenHash    string
	maxUses      *int
	consumedHash string
}

func (r *recordingRepo) CreateDirect(_ context.Context, _, _, _ string, expiresAt *time.Time) (*invitation.Direct, error) {
	r.expiresAt = expiresAt
	return &invitation.Direct{}, nil
}
func (*recordingRepo) ListReceived(context.Context, string) ([]invitation.Direct, error) {
	return nil, nil
}
func (*recordingRepo) ListForProject(context.Context, string, string) ([]invitation.Direct, error) {
	return nil, nil
}
func (*recordingRepo) Accept(context.Context, string, string, time.Time) (string, error) {
	return "", nil
}
func (*recordingRepo) Decline(context.Context, string, string, time.Time) error        { return nil }
func (*recordingRepo) Cancel(context.Context, string, string, string, time.Time) error { return nil }
func (r *recordingRepo) CreateCredential(_ context.Context, _, _, hash string, expires *time.Time, max *int) (*invitation.Credential, error) {
	r.tokenHash = hash
	r.expiresAt = expires
	r.maxUses = max
	return &invitation.Credential{}, nil
}
func (*recordingRepo) ListCredentials(context.Context, string, string) ([]invitation.Credential, error) {
	return nil, nil
}
func (*recordingRepo) RevokeCredential(context.Context, string, string, string, time.Time) error {
	return nil
}
func (r *recordingRepo) ConsumeCredential(_ context.Context, hash, _ string, _ time.Time) (string, error) {
	r.consumedHash = hash
	return "project-1", nil
}

func TestCredentialSecretIsRandomHashedAndURLConsumable(t *testing.T) {
	repo := &recordingRepo{}
	svc := &invitation.Service{Repo: repo}
	item, err := svc.CreateCredential(context.Background(), "p", "u", "7d", nil)
	if err != nil {
		t.Fatal(err)
	}
	if item.Secret == "" {
		t.Fatal("expected returned code")
	}
	if repo.tokenHash == item.Secret {
		t.Fatal("repository received raw secret")
	}
	if repo.tokenHash != invitation.HashCredentialSecret(item.Secret) {
		t.Fatal("repository hash mismatch")
	}
	other, err := invitation.NewCredentialSecret()
	if err != nil {
		t.Fatal(err)
	}
	if other == item.Secret {
		t.Fatal("two generated secrets matched")
	}
	projectID, err := svc.JoinWithCredential(context.Background(), "https://hub.example/invite/"+item.Secret, "member")
	if err != nil || projectID != "project-1" {
		t.Fatalf("join: %s %v", projectID, err)
	}
	if repo.consumedHash != repo.tokenHash {
		t.Fatal("URL did not resolve to credential hash")
	}
}

func TestCredentialRejectsInvalidUseLimit(t *testing.T) {
	zero := 0
	svc := &invitation.Service{Repo: &recordingRepo{}}
	if _, err := svc.CreateCredential(context.Background(), "p", "u", "7d", &zero); !errors.Is(err, invitation.ErrInvalidUseLimit) {
		t.Fatalf("expected ErrInvalidUseLimit, got %v", err)
	}
}

func TestCreateDirectExpirationChoices(t *testing.T) {
	now := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		choice   string
		duration time.Duration
		never    bool
	}{{"1h", time.Hour, false}, {"1d", 24 * time.Hour, false}, {"7d", 7 * 24 * time.Hour, false}, {"30d", 30 * 24 * time.Hour, false}, {"never", 0, true}} {
		t.Run(tc.choice, func(t *testing.T) {
			repo := &recordingRepo{}
			svc := &invitation.Service{Repo: repo, Now: func() time.Time { return now }}
			if _, err := svc.CreateDirect(context.Background(), "p", "u", "recipient", tc.choice); err != nil {
				t.Fatal(err)
			}
			if tc.never {
				if repo.expiresAt != nil {
					t.Fatalf("expected no expiry")
				}
			} else if repo.expiresAt == nil || !repo.expiresAt.Equal(now.Add(tc.duration)) {
				t.Fatalf("unexpected expiry: %v", repo.expiresAt)
			}
		})
	}
}

func TestCreateDirectRejectsInvalidExpiration(t *testing.T) {
	svc := &invitation.Service{Repo: &recordingRepo{}}
	if _, err := svc.CreateDirect(context.Background(), "p", "u", "recipient", "weekly"); !errors.Is(err, invitation.ErrInvalidExpiration) {
		t.Fatalf("expected ErrInvalidExpiration, got %v", err)
	}
}
