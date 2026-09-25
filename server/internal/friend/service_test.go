package friend_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/friend"
	"github.com/itswskkk/KMJG-Hub/server/internal/friend/friendtest"
)

type recordingPublisher struct {
	mu     sync.Mutex
	events []string
}

func (p *recordingPublisher) add(e string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, e)
}

func (p *recordingPublisher) PublishRequestSent(friend.FriendRequest)      { p.add("sent") }
func (p *recordingPublisher) PublishRequestAccepted(friend.FriendRequest)  { p.add("accepted") }
func (p *recordingPublisher) PublishRequestDeclined(friend.FriendRequest)  { p.add("declined") }
func (p *recordingPublisher) PublishRequestCancelled(friend.FriendRequest) { p.add("cancelled") }
func (p *recordingPublisher) PublishFriendRemoved(string, string)          { p.add("removed") }
func (p *recordingPublisher) PublishBlocked(string, string)                { p.add("blocked") }
func (p *recordingPublisher) PublishUnblocked(string, string)              { p.add("unblocked") }

func newService() (*friend.Service, *recordingPublisher) {
	users := friendtest.StaticUsers{"alice": "alice", "bob": "bob", "carol": "carol"}
	pub := &recordingPublisher{}
	return &friend.Service{Repo: friendtest.NewMemory(users), Publisher: pub}, pub
}

var ctx = context.Background()

func mustSend(t *testing.T, s *friend.Service, from, to string) *friend.FriendRequest {
	t.Helper()
	req, err := s.SendRequest(ctx, from, to)
	if err != nil {
		t.Fatalf("SendRequest(%s->%s): %v", from, to, err)
	}
	return req
}

func mustBeFriends(t *testing.T, s *friend.Service, a, b string, want bool) {
	t.Helper()
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		got, err := s.AreFriends(ctx, pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("AreFriends(%s,%s)=%v, want %v", pair[0], pair[1], got, want)
		}
	}
}

func TestCannotSendRequestToSelf(t *testing.T) {
	s, _ := newService()
	if _, err := s.SendRequest(ctx, "alice", "alice"); !errors.Is(err, friend.ErrSelfRequest) {
		t.Fatalf("expected ErrSelfRequest, got %v", err)
	}
}

func TestSendRequestRequiresRecipient(t *testing.T) {
	s, _ := newService()
	var v *friend.ValidationError
	if _, err := s.SendRequest(ctx, "alice", " "); !errors.As(err, &v) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if _, err := s.SendRequest(ctx, "alice", "nobody"); !errors.Is(err, friend.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDuplicatePendingRequestRejectedInBothDirections(t *testing.T) {
	s, _ := newService()
	mustSend(t, s, "alice", "bob")
	if _, err := s.SendRequest(ctx, "alice", "bob"); !errors.Is(err, friend.ErrRequestPending) {
		t.Fatalf("duplicate: expected ErrRequestPending, got %v", err)
	}
	if _, err := s.SendRequest(ctx, "bob", "alice"); !errors.Is(err, friend.ErrRequestPending) {
		t.Fatalf("reverse: expected ErrRequestPending, got %v", err)
	}
}

func TestAcceptCreatesSymmetricFriendship(t *testing.T) {
	s, pub := newService()
	req := mustSend(t, s, "alice", "bob")
	accepted, err := s.AcceptRequest(ctx, "bob", req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != friend.StatusAccepted {
		t.Fatalf("status=%s", accepted.Status)
	}
	mustBeFriends(t, s, "alice", "bob", true)

	for _, user := range []string{"alice", "bob"} {
		list, err := s.ListFriends(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 {
			t.Fatalf("%s friends=%+v", user, list)
		}
	}
	if _, err := s.SendRequest(ctx, "bob", "alice"); !errors.Is(err, friend.ErrAlreadyFriends) {
		t.Fatalf("expected ErrAlreadyFriends, got %v", err)
	}
	if len(pub.events) != 2 || pub.events[0] != "sent" || pub.events[1] != "accepted" {
		t.Fatalf("events=%v", pub.events)
	}
}

func TestOnlyRecipientCanAcceptOrDecline(t *testing.T) {
	s, _ := newService()
	req := mustSend(t, s, "alice", "bob")
	if _, err := s.AcceptRequest(ctx, "alice", req.ID); !errors.Is(err, friend.ErrForbidden) {
		t.Fatalf("sender accept: expected ErrForbidden, got %v", err)
	}
	if _, err := s.DeclineRequest(ctx, "alice", req.ID); !errors.Is(err, friend.ErrForbidden) {
		t.Fatalf("sender decline: expected ErrForbidden, got %v", err)
	}
	if _, err := s.AcceptRequest(ctx, "carol", req.ID); !errors.Is(err, friend.ErrNotFound) {
		t.Fatalf("outsider accept: expected ErrNotFound, got %v", err)
	}
	if _, err := s.DeclineRequest(ctx, "bob", req.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptRequest(ctx, "bob", req.ID); !errors.Is(err, friend.ErrNotPending) {
		t.Fatalf("accept after decline: expected ErrNotPending, got %v", err)
	}
	mustBeFriends(t, s, "alice", "bob", false)

	// A declined request does not block a later one.
	again := mustSend(t, s, "alice", "bob")
	if again.Status != friend.StatusPending {
		t.Fatalf("resend status=%s", again.Status)
	}
}

func TestOnlySenderCanCancel(t *testing.T) {
	s, _ := newService()
	req := mustSend(t, s, "alice", "bob")
	if err := s.CancelRequest(ctx, "bob", req.ID); !errors.Is(err, friend.ErrForbidden) {
		t.Fatalf("recipient cancel: expected ErrForbidden, got %v", err)
	}
	if err := s.CancelRequest(ctx, "alice", req.ID); err != nil {
		t.Fatal(err)
	}
	incoming, _ := s.ListIncomingRequests(ctx, "bob")
	if len(incoming) != 0 {
		t.Fatalf("incoming after cancel=%+v", incoming)
	}
}

func TestBlockingRemovesFriendship(t *testing.T) {
	s, pub := newService()
	req := mustSend(t, s, "alice", "bob")
	if _, err := s.AcceptRequest(ctx, "bob", req.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Block(ctx, "alice", "bob"); err != nil {
		t.Fatal(err)
	}
	mustBeFriends(t, s, "alice", "bob", false)
	if pub.events[len(pub.events)-1] != "blocked" {
		t.Fatalf("events=%v", pub.events)
	}

	// Unblocking does not restore the friendship (docs/PRD.md § Blocking).
	if err := s.Unblock(ctx, "alice", "bob"); err != nil {
		t.Fatal(err)
	}
	mustBeFriends(t, s, "alice", "bob", false)
}

func TestBlockingPreventsRequestsInBothDirections(t *testing.T) {
	s, _ := newService()
	if _, err := s.Block(ctx, "alice", "bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SendRequest(ctx, "bob", "alice"); !errors.Is(err, friend.ErrBlocked) {
		t.Fatalf("blocked->blocker: expected ErrBlocked, got %v", err)
	}
	if _, err := s.SendRequest(ctx, "alice", "bob"); !errors.Is(err, friend.ErrBlocked) {
		t.Fatalf("blocker->blocked: expected ErrBlocked, got %v", err)
	}
	if err := s.Unblock(ctx, "alice", "bob"); err != nil {
		t.Fatal(err)
	}
	mustSend(t, s, "bob", "alice")
}

func TestBlockingDropsPendingRequests(t *testing.T) {
	s, _ := newService()
	req := mustSend(t, s, "bob", "alice")
	if _, err := s.Block(ctx, "alice", "bob"); err != nil {
		t.Fatal(err)
	}
	incoming, _ := s.ListIncomingRequests(ctx, "alice")
	if len(incoming) != 0 {
		t.Fatalf("incoming after block=%+v", incoming)
	}
	if _, err := s.AcceptRequest(ctx, "alice", req.ID); !errors.Is(err, friend.ErrNotFound) {
		t.Fatalf("accept after block: expected ErrNotFound, got %v", err)
	}
}

func TestBlockIsIdempotentAndListed(t *testing.T) {
	s, _ := newService()
	if _, err := s.Block(ctx, "alice", "alice"); !errors.Is(err, friend.ErrSelfBlock) {
		t.Fatalf("self block: expected ErrSelfBlock, got %v", err)
	}
	first, err := s.Block(ctx, "alice", "bob")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Block(ctx, "alice", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("re-block created a new block: %s vs %s", first.ID, second.ID)
	}
	list, _ := s.ListBlocked(ctx, "alice")
	if len(list) != 1 || list[0].BlockedID != "bob" {
		t.Fatalf("blocked=%+v", list)
	}
	if blocked, _ := s.IsBlocked(ctx, "bob", "alice"); blocked {
		t.Fatal("block must be directional")
	}
	if either, _ := s.EitherBlocked(ctx, "bob", "alice"); !either {
		t.Fatal("EitherBlocked should see alice's block")
	}
	if err := s.Unblock(ctx, "alice", "carol"); !errors.Is(err, friend.ErrNotFound) {
		t.Fatalf("unblock non-blocked: expected ErrNotFound, got %v", err)
	}
}

func TestRemoveFriendship(t *testing.T) {
	s, _ := newService()
	req := mustSend(t, s, "alice", "bob")
	if _, err := s.AcceptRequest(ctx, "bob", req.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveFriendship(ctx, "bob", "alice"); err != nil {
		t.Fatal(err)
	}
	mustBeFriends(t, s, "alice", "bob", false)
	if err := s.RemoveFriendship(ctx, "bob", "alice"); !errors.Is(err, friend.ErrNotFound) {
		t.Fatalf("remove again: expected ErrNotFound, got %v", err)
	}
}
