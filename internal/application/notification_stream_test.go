package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"note-app/internal/domain"
)

type notificationStreamBrokerStub struct {
	subscribeFn func(context.Context, string) (NotificationStreamSubscription, error)
}

func (b notificationStreamBrokerStub) Subscribe(ctx context.Context, userID string) (NotificationStreamSubscription, error) {
	if b.subscribeFn != nil {
		return b.subscribeFn(ctx, userID)
	}
	return nil, nil
}

type notificationStreamSubscriptionStub struct {
	events  chan domain.NotificationStreamSignal
	closeFn func() error
	once    sync.Once
}

func (s *notificationStreamSubscriptionStub) Events() <-chan domain.NotificationStreamSignal {
	return s.events
}

func (s *notificationStreamSubscriptionStub) Close() error {
	if s.closeFn != nil {
		return s.closeFn()
	}
	s.once.Do(func() {
		if s.events != nil {
			close(s.events)
		}
	})
	return nil
}

func TestNotificationStreamServiceOpen(t *testing.T) {
	t.Run("emits unread count changes and invalidations", func(t *testing.T) {
		counts := []int64{3, 5, 5, 4}
		repo := notificationRepoStub{
			getUnreadFn: func(context.Context, string) (int64, error) {
				if len(counts) == 0 {
					return 0, errors.New("unexpected unread count lookup")
				}
				value := counts[0]
				counts = counts[1:]
				return value, nil
			},
		}
		subscription := &notificationStreamSubscriptionStub{events: make(chan domain.NotificationStreamSignal, 4)}
		service := NewNotificationStreamService(repo, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "user-1", Email: "user@example.com"}, nil
		}}, notificationStreamBrokerStub{
			subscribeFn: func(context.Context, string) (NotificationStreamSubscription, error) {
				return subscription, nil
			},
		}, time.Now)

		session, err := service.Open(context.Background(), "user-1")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		if got := session.InitialUnreadCount(); got != 3 {
			t.Fatalf("unexpected initial unread count: %d", got)
		}

		emit := func(expectedType string, expectedUnread *int64) NotificationStreamEvent {
			t.Helper()
			select {
			case event := <-session.Events():
				if event.Type != expectedType {
					t.Fatalf("unexpected event type: got %s want %s", event.Type, expectedType)
				}
				if expectedUnread == nil {
					if event.UnreadCount != nil {
						t.Fatalf("expected nil unread count, got %d", *event.UnreadCount)
					}
				} else if event.UnreadCount == nil || *event.UnreadCount != *expectedUnread {
					t.Fatalf("unexpected unread count: got %+v want %d", event.UnreadCount, *expectedUnread)
				}
				if event.SentAt.IsZero() {
					t.Fatal("expected sent_at to be set")
				}
				return event
			case <-time.After(2 * time.Second):
				t.Fatalf("timed out waiting for %s event", expectedType)
			}
			return NotificationStreamEvent{}
		}

		subscription.events <- domain.NotificationStreamSignal{UserID: "user-1", Reason: domain.NotificationStreamReasonNotificationsChanged, SentAt: time.Now().UTC()}
		emit("unread_count", int64Ptr(5))
		emit("inbox_invalidated", nil)

		subscription.events <- domain.NotificationStreamSignal{UserID: "user-1", Reason: domain.NotificationStreamReasonNotificationsChanged, SentAt: time.Now().UTC()}
		emit("inbox_invalidated", nil)

		subscription.events <- domain.NotificationStreamSignal{UserID: "user-1", Reason: domain.NotificationStreamReasonNotificationsChanged, SentAt: time.Now().UTC()}
		emit("unread_count", int64Ptr(4))
		emit("inbox_invalidated", nil)

		if err := session.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	t.Run("rejects unknown actor", func(t *testing.T) {
		service := NewNotificationStreamService(notificationRepoStub{}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{}, domain.ErrNotFound
		}}, notificationStreamBrokerStub{}, time.Now)

		if _, err := service.Open(context.Background(), "missing"); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("expected unauthorized, got %v", err)
		}
	})

	t.Run("fails when unread count lookup fails", func(t *testing.T) {
		service := NewNotificationStreamService(notificationRepoStub{
			getUnreadFn: func(context.Context, string) (int64, error) {
				return 0, errors.New("count failed")
			},
		}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "user-1", Email: "user@example.com"}, nil
		}}, notificationStreamBrokerStub{}, time.Now)

		if _, err := service.Open(context.Background(), "user-1"); err == nil || err.Error() != "count failed" {
			t.Fatalf("expected unread count failure, got %v", err)
		}
	})

	t.Run("fails when broker subscription fails", func(t *testing.T) {
		service := NewNotificationStreamService(notificationRepoStub{
			getUnreadFn: func(context.Context, string) (int64, error) {
				return 1, nil
			},
		}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "user-1", Email: "user@example.com"}, nil
		}}, notificationStreamBrokerStub{
			subscribeFn: func(context.Context, string) (NotificationStreamSubscription, error) {
				return nil, errors.New("subscribe failed")
			},
		}, time.Now)

		if _, err := service.Open(context.Background(), "user-1"); err == nil || err.Error() != "subscribe failed" {
			t.Fatalf("expected subscribe failure, got %v", err)
		}
	})

	t.Run("stops delivery when post-open count lookup fails", func(t *testing.T) {
		countCalls := 0
		subscription := &notificationStreamSubscriptionStub{events: make(chan domain.NotificationStreamSignal, 1)}
		service := NewNotificationStreamService(notificationRepoStub{
			getUnreadFn: func(context.Context, string) (int64, error) {
				countCalls++
				if countCalls == 1 {
					return 2, nil
				}
				return 0, errors.New("count failed")
			},
		}, authUserRepoStub{getByIDFn: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "user-1", Email: "user@example.com"}, nil
		}}, notificationStreamBrokerStub{
			subscribeFn: func(context.Context, string) (NotificationStreamSubscription, error) {
				return subscription, nil
			},
		}, time.Now)

		session, err := service.Open(context.Background(), "user-1")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		subscription.events <- domain.NotificationStreamSignal{UserID: "user-1", Reason: domain.NotificationStreamReasonNotificationsChanged, SentAt: time.Now().UTC()}

		select {
		case _, ok := <-session.Events():
			if ok {
				t.Fatal("expected session delivery to stop after unread count failure")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("expected session to close after unread count failure")
		}
	})
}

func int64Ptr(value int64) *int64 {
	return &value
}
