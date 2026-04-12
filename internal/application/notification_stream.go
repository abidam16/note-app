package application

import (
	"context"
	"errors"
	"sync"
	"time"

	"note-app/internal/domain"
)

type NotificationStreamSubscription = domain.NotificationStreamSubscription

type NotificationStreamBroker interface {
	Subscribe(ctx context.Context, userID string) (NotificationStreamSubscription, error)
}

type NotificationStreamEvent struct {
	Type        string    `json:"type"`
	UnreadCount *int64    `json:"unread_count,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	SentAt      time.Time `json:"sent_at"`
}

type NotificationStreamSession interface {
	InitialUnreadCount() int64
	Events() <-chan NotificationStreamEvent
	Close() error
}

type NotificationStreamService struct {
	users         UserRepository
	notifications NotificationRepository
	broker        NotificationStreamBroker
	now           func() time.Time
}

func NewNotificationStreamService(notifications NotificationRepository, users UserRepository, broker NotificationStreamBroker, now func() time.Time) NotificationStreamService {
	if now == nil {
		now = time.Now
	}
	return NotificationStreamService{
		users:         users,
		notifications: notifications,
		broker:        broker,
		now:           now,
	}
}

func (s NotificationStreamService) Open(ctx context.Context, actorID string) (NotificationStreamSession, error) {
	if s.broker == nil {
		return nil, errors.New("notification stream broker is not configured")
	}
	if _, err := s.users.GetByID(ctx, actorID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}

	initialUnreadCount, err := s.notifications.GetUnreadCount(ctx, actorID)
	if err != nil {
		return nil, err
	}

	subscription, err := s.broker.Subscribe(ctx, actorID)
	if err != nil {
		return nil, err
	}

	session := &notificationStreamSession{
		initialUnreadCount: initialUnreadCount,
		events:             make(chan NotificationStreamEvent, 8),
		subscription:       subscription,
		notifications:      s.notifications,
		userID:             actorID,
		now:                s.now,
	}
	session.start(ctx)
	return session, nil
}

type notificationStreamSession struct {
	initialUnreadCount int64
	events             chan NotificationStreamEvent
	subscription       NotificationStreamSubscription
	notifications      NotificationRepository
	userID             string
	now                func() time.Time

	once sync.Once
	done chan struct{}
}

func (s *notificationStreamSession) start(ctx context.Context) {
	s.done = make(chan struct{})
	go s.run(ctx)
}

func (s *notificationStreamSession) run(ctx context.Context) {
	defer close(s.events)
	defer close(s.done)
	defer func() {
		if s.subscription != nil {
			_ = s.subscription.Close()
		}
	}()

	lastUnreadCount := s.initialUnreadCount
	for {
		select {
		case <-ctx.Done():
			return
		case signal, ok := <-s.subscription.Events():
			if !ok {
				return
			}

			unreadCount, err := s.notifications.GetUnreadCount(ctx, s.userID)
			if err != nil {
				return
			}

			if unreadCount != lastUnreadCount {
				lastUnreadCount = unreadCount
				s.emit(NotificationStreamEvent{
					Type:        "unread_count",
					UnreadCount: int64PtrValue(unreadCount),
					SentAt:      s.now().UTC(),
				})
			}

			reason := string(signal.Reason)
			if reason == "" {
				reason = string(domain.NotificationStreamReasonNotificationsChanged)
			}
			s.emit(NotificationStreamEvent{
				Type:   "inbox_invalidated",
				Reason: reason,
				SentAt: s.now().UTC(),
			})
		}
	}
}

func (s *notificationStreamSession) emit(event NotificationStreamEvent) {
	select {
	case s.events <- event:
	case <-s.done:
	}
}

func (s *notificationStreamSession) InitialUnreadCount() int64 {
	return s.initialUnreadCount
}

func (s *notificationStreamSession) Events() <-chan NotificationStreamEvent {
	return s.events
}

func (s *notificationStreamSession) Close() error {
	s.once.Do(func() {
		if s.subscription != nil {
			_ = s.subscription.Close()
		}
	})
	<-s.done
	return nil
}

func int64PtrValue(value int64) *int64 {
	return &value
}
