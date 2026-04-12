package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"note-app/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const notificationStreamChannelName = "notification_stream"

type notificationStreamPool interface {
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type NotificationStreamBroker struct {
	pool notificationStreamPool

	mu          sync.Mutex
	subscribers map[string]map[*notificationStreamSubscription]struct{}
	started     bool
	startFn     func(chan<- error)
}

type notificationStreamSubscription struct {
	broker *NotificationStreamBroker
	userID string
	events chan domain.NotificationStreamSignal
	mu     sync.Mutex
	closed bool
}

func NewNotificationStreamBroker(pool *pgxpool.Pool) *NotificationStreamBroker {
	return newNotificationStreamBroker(pool)
}

func newNotificationStreamBroker(pool notificationStreamPool) *NotificationStreamBroker {
	broker := &NotificationStreamBroker{
		pool:        pool,
		subscribers: map[string]map[*notificationStreamSubscription]struct{}{},
	}
	broker.startFn = broker.runListener
	return broker
}

func (b *NotificationStreamBroker) Publish(ctx context.Context, signal domain.NotificationStreamSignal) error {
	if b == nil || b.pool == nil {
		return errors.New("notification stream broker is not configured")
	}

	payload, err := json.Marshal(signal)
	if err != nil {
		return fmt.Errorf("marshal notification stream signal: %w", err)
	}

	if _, err := b.pool.Exec(ctx, "SELECT pg_notify($1, $2)", notificationStreamChannelName, string(payload)); err != nil {
		return fmt.Errorf("publish notification stream signal: %w", err)
	}

	return nil
}

func (b *NotificationStreamBroker) Subscribe(_ context.Context, userID string) (domain.NotificationStreamSubscription, error) {
	if b == nil || b.pool == nil {
		return nil, errors.New("notification stream broker is not configured")
	}

	subscription := b.subscribeLocal(userID)
	if err := b.ensureListener(); err != nil {
		_ = subscription.Close()
		return nil, err
	}

	return subscription, nil
}

func (b *NotificationStreamBroker) ensureListener() error {
	b.mu.Lock()
	if b.started {
		b.mu.Unlock()
		return nil
	}
	b.started = true
	b.mu.Unlock()

	ready := make(chan error, 1)
	go b.startFn(ready)
	if err := <-ready; err != nil {
		return err
	}
	return nil
}

func (b *NotificationStreamBroker) runListener(ready chan<- error) {
	ctx := context.Background()
	conn, err := b.pool.Acquire(ctx)
	if err != nil {
		ready <- err
		b.failListener(err)
		return
	}
	defer conn.Release()

	if _, err := conn.Conn().Exec(ctx, "LISTEN "+notificationStreamChannelName); err != nil {
		ready <- err
		b.failListener(err)
		return
	}
	ready <- nil

	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			b.failListener(err)
			return
		}
		if notification == nil {
			continue
		}

		signal, ok := decodeNotificationStreamSignal([]byte(notification.Payload))
		if !ok {
			continue
		}
		b.dispatch(signal)
	}
}

func (b *NotificationStreamBroker) failListener(err error) {
	if err == nil {
		return
	}
	b.mu.Lock()
	b.started = false
	subscribers := b.subscribers
	b.subscribers = map[string]map[*notificationStreamSubscription]struct{}{}
	b.mu.Unlock()

	for _, userSubscriptions := range subscribers {
		for subscription := range userSubscriptions {
			subscription.closeFromBroker()
		}
	}
}

func (b *NotificationStreamBroker) subscribeLocal(userID string) *notificationStreamSubscription {
	subscription := &notificationStreamSubscription{
		broker: b,
		userID: userID,
		events: make(chan domain.NotificationStreamSignal, 8),
	}

	b.mu.Lock()
	if b.subscribers[userID] == nil {
		b.subscribers[userID] = map[*notificationStreamSubscription]struct{}{}
	}
	b.subscribers[userID][subscription] = struct{}{}
	b.mu.Unlock()

	return subscription
}

func (b *NotificationStreamBroker) removeSubscription(subscription *notificationStreamSubscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	userSubscriptions, ok := b.subscribers[subscription.userID]
	if !ok {
		return
	}
	delete(userSubscriptions, subscription)
	if len(userSubscriptions) == 0 {
		delete(b.subscribers, subscription.userID)
	}
}

func (b *NotificationStreamBroker) dispatch(signal domain.NotificationStreamSignal) {
	b.mu.Lock()
	subscribers := b.subscribers[signal.UserID]
	targets := make([]*notificationStreamSubscription, 0, len(subscribers))
	for subscription := range subscribers {
		targets = append(targets, subscription)
	}
	b.mu.Unlock()

	for _, subscription := range targets {
		subscription.deliver(signal)
	}
}

func decodeNotificationStreamSignal(payload []byte) (domain.NotificationStreamSignal, bool) {
	var signal domain.NotificationStreamSignal
	if err := json.Unmarshal(payload, &signal); err != nil {
		return domain.NotificationStreamSignal{}, false
	}
	if strings.TrimSpace(signal.UserID) == "" {
		return domain.NotificationStreamSignal{}, false
	}
	if signal.Reason != domain.NotificationStreamReasonNotificationsChanged {
		return domain.NotificationStreamSignal{}, false
	}
	if signal.SentAt.IsZero() {
		return domain.NotificationStreamSignal{}, false
	}
	return signal, true
}

func (s *notificationStreamSubscription) Events() <-chan domain.NotificationStreamSignal {
	return s.events
}

func (s *notificationStreamSubscription) Close() error {
	s.closeFromBroker()
	return nil
}

func (s *notificationStreamSubscription) deliver(signal domain.NotificationStreamSignal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.events <- signal:
	default:
	}
}

func (s *notificationStreamSubscription) closeFromBroker() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.events)
	s.mu.Unlock()

	if s.broker != nil {
		s.broker.removeSubscription(s)
	}
}
