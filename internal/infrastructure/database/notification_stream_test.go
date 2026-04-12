package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"note-app/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeNotificationStreamPool struct {
	execFn func(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func (p *fakeNotificationStreamPool) Acquire(context.Context) (*pgxpool.Conn, error) {
	return nil, nil
}

func (p *fakeNotificationStreamPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if p.execFn != nil {
		return p.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func TestPostgresNotificationStreamBroker(t *testing.T) {
	t.Run("routes signals to matching user only", func(t *testing.T) {
		broker := newNotificationStreamBroker(&fakeNotificationStreamPool{})
		alice := broker.subscribeLocal("user-a")
		bob := broker.subscribeLocal("user-b")
		defer alice.Close()
		defer bob.Close()

		broker.dispatch(domain.NotificationStreamSignal{UserID: "user-a", Reason: domain.NotificationStreamReasonNotificationsChanged, SentAt: time.Now().UTC()})

		select {
		case signal := <-alice.Events():
			if signal.UserID != "user-a" {
				t.Fatalf("unexpected signal user: %+v", signal)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("expected signal for matching user")
		}

		select {
		case <-bob.Events():
			t.Fatal("expected other user subscriber to stay quiet")
		default:
		}
	})

	t.Run("fanouts to multiple local subscribers", func(t *testing.T) {
		broker := newNotificationStreamBroker(&fakeNotificationStreamPool{})
		first := broker.subscribeLocal("user-a")
		second := broker.subscribeLocal("user-a")
		defer first.Close()
		defer second.Close()

		broker.dispatch(domain.NotificationStreamSignal{UserID: "user-a", Reason: domain.NotificationStreamReasonNotificationsChanged, SentAt: time.Now().UTC()})

		for idx, sub := range []*notificationStreamSubscription{first, second} {
			select {
			case signal := <-sub.Events():
				if signal.UserID != "user-a" {
					t.Fatalf("unexpected signal on subscriber %d: %+v", idx, signal)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("expected signal on subscriber %d", idx)
			}
		}
	})

	t.Run("ignores malformed payloads", func(t *testing.T) {
		if _, ok := decodeNotificationStreamSignal([]byte(`{"user_id":"","reason":"notifications_changed","sent_at":"2026-04-04T10:00:00Z"}`)); ok {
			t.Fatal("expected blank user payload to be rejected")
		}
		if _, ok := decodeNotificationStreamSignal([]byte(`{"user_id":"user-a","reason":"notifications_changed","sent_at":"bad"}`)); ok {
			t.Fatal("expected malformed timestamp to be rejected")
		}
	})

	t.Run("unsubscribed clients stop receiving events", func(t *testing.T) {
		broker := newNotificationStreamBroker(&fakeNotificationStreamPool{})
		sub := broker.subscribeLocal("user-a")
		if err := sub.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		broker.dispatch(domain.NotificationStreamSignal{UserID: "user-a", Reason: domain.NotificationStreamReasonNotificationsChanged, SentAt: time.Now().UTC()})
		select {
		case _, ok := <-sub.Events():
			if ok {
				t.Fatal("expected closed subscriber to remain quiet")
			}
		default:
		}
	})

	t.Run("publish failures are returned", func(t *testing.T) {
		wantErr := errors.New("notify failed")
		broker := newNotificationStreamBroker(&fakeNotificationStreamPool{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, wantErr
			},
		})

		if err := broker.Publish(context.Background(), domain.NotificationStreamSignal{UserID: "user-a", Reason: domain.NotificationStreamReasonNotificationsChanged, SentAt: time.Now().UTC()}); !errors.Is(err, wantErr) {
			t.Fatalf("expected publish failure, got %v", err)
		}
	})

	t.Run("retries listener startup after a prior failure", func(t *testing.T) {
		broker := newNotificationStreamBroker(&fakeNotificationStreamPool{})
		startCalls := 0
		broker.startFn = func(ready chan<- error) {
			startCalls++
			if startCalls == 1 {
				err := errors.New("listen failed")
				ready <- err
				broker.failListener(err)
				return
			}
			ready <- nil
		}

		if _, err := broker.Subscribe(context.Background(), "user-a"); err == nil || err.Error() != "listen failed" {
			t.Fatalf("expected initial listen failure, got %v", err)
		}

		subscription, err := broker.Subscribe(context.Background(), "user-a")
		if err != nil {
			t.Fatalf("expected subscribe retry to succeed, got %v", err)
		}
		if startCalls != 2 {
			t.Fatalf("expected two listener start attempts, got %d", startCalls)
		}
		if err := subscription.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
}
