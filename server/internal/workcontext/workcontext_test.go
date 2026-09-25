package workcontext_test

import (
	"context"
	"errors"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/workcontext"
)

type repo struct {
	values  []workcontext.Context
	updated *workcontext.Context
}

func (r *repo) List(context.Context, string, string) ([]workcontext.Context, error) {
	return append([]workcontext.Context(nil), r.values...), nil
}
func (r *repo) Upsert(_ context.Context, projectID, userID string, working bool, mode, branch string) (*workcontext.Context, error) {
	r.updated = &workcontext.Context{ProjectID: projectID, UserID: userID, Working: working, StatusMode: mode, CurrentBranch: branch}
	return r.updated, nil
}

type online map[string]bool

func (o online) IsOnline(userID string) bool { return o[userID] }

type membership struct{ ids []string }

func (m membership) MemberUserIDs(context.Context, string) ([]string, error) { return m.ids, nil }

type publisher struct{ recipients []string }

func (p *publisher) PublishWorkContext(userID string, _ workcontext.Context) {
	p.recipients = append(p.recipients, userID)
}

func TestListHidesOfflinePeersButReturnsViewerOwnContext(t *testing.T) {
	r := &repo{values: []workcontext.Context{{UserID: "viewer", Working: false, StatusMode: "manual"}, {UserID: "online", Working: true}, {UserID: "offline", Working: true}}}
	svc := workcontext.Service{Repo: r, Online: online{"online": true}}
	values, err := svc.List(context.Background(), "viewer", "project")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].UserID != "viewer" || values[1].UserID != "online" {
		t.Fatalf("visible contexts=%+v", values)
	}
}

func TestUpdateValidatesAndPublishesToAuthoritativeMembers(t *testing.T) {
	r := &repo{}
	p := &publisher{}
	svc := workcontext.Service{Repo: r, Membership: membership{[]string{"u1", "u2"}}, Publisher: p}
	value, err := svc.Update(context.Background(), "u1", "p1", false, "manual", " feature/x ")
	if err != nil {
		t.Fatal(err)
	}
	if value.CurrentBranch != "feature/x" || value.StatusMode != "manual" {
		t.Fatalf("value=%+v", value)
	}
	if len(p.recipients) != 2 {
		t.Fatalf("recipients=%v", p.recipients)
	}
	_, err = svc.Update(context.Background(), "u1", "p1", true, "invalid", "")
	var validation *workcontext.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
