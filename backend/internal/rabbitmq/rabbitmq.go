// Package rabbitmq is the control plane: a thin RabbitMQ client plus the three
// durable queues and the three JSON messages that flow over them between the
// lobby and the workers.
package rabbitmq

import (
	"context"
	"encoding/json"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// The three queues. Start fans out to competing worker consumers; the other two
// carry replies back to the lobby.
const (
	QueueStart    = "tictactoe.start"    // lobby -> workers (competing consumers)
	QueueAssigned = "tictactoe.assigned" // worker -> lobby
	QueueResult   = "tictactoe.result"   // worker -> lobby
)

// Fields common to several messages are defined once in a small struct and
// embedded, so a field and its JSON tag never drift between messages. encoding/
// json inlines embedded struct fields, so the wire format stays flat.

// GameRef identifies the game a message concerns.
type GameRef struct {
	GameID string `json:"game_id"`
}

// Players names the two sides of a game.
type Players struct {
	PlayerX string `json:"player_x"`
	PlayerO string `json:"player_o"`
}

// StartGame asks a worker to host a new game (published to QueueStart).
type StartGame struct {
	GameRef
	Players
	BoardSize int `json:"board_size"`
	WinLength int `json:"win_length"`
}

// GameAssigned tells the lobby which worker now hosts a game (QueueAssigned).
type GameAssigned struct {
	GameRef
	WorkerAddr string `json:"worker_addr"`
}

// GameResult reports the final outcome of a game to the lobby (QueueResult).
type GameResult struct {
	GameRef
	Players
	WinnerID string `json:"winner_id"`
	Draw     bool   `json:"draw"`
}

// Publisher is the one thing the lobby and workers need from the broker: publish a
// message to a queue. Defined here, next to the Broker that satisfies it, so both
// callers share a single definition.
type Publisher interface {
	Publish(ctx context.Context, queue string, msg any) error
}

// Broker is a connection to RabbitMQ over a single channel.
type Broker struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

// Connect dials the broker (retrying until it is ready or ctx is cancelled) and
// declares the queues.
func Connect(ctx context.Context, url string) (*Broker, error) {
	conn, err := dial(ctx, url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}
	// Prefetch 1: a worker holds at most one unacked message, so an idle worker
	// takes the next game rather than one worker hoarding the queue.
	if err := ch.Qos(1, 0, false); err != nil {
		conn.Close()
		return nil, err
	}
	for _, q := range []string{QueueStart, QueueAssigned, QueueResult} {
		if _, err := ch.QueueDeclare(q, true, false, false, false, nil); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return &Broker{conn: conn, ch: ch}, nil
}

func dial(ctx context.Context, url string) (*amqp.Connection, error) {
	for {
		conn, err := amqp.Dial(url)
		if err == nil {
			return conn, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (b *Broker) Publish(ctx context.Context, queue string, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return b.ch.PublishWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}

// Handler processes one message. Returning nil acks it; returning an error nacks
// it with requeue, so the broker redelivers it (to this or another consumer).
type Handler func(ctx context.Context, body []byte) error

// Consume runs handle for each message on queue: it acks on success and requeues
// on error. Requeue-on-error is how a worker "refuses" work it cannot take right
// now — the broker simply offers the message to another consumer.
func (b *Broker) Consume(queue string, handle Handler) error {
	deliveries, err := b.ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	go func() {
		for d := range deliveries {
			if err := handle(context.Background(), d.Body); err != nil {
				_ = d.Nack(false, true)
				continue
			}
			_ = d.Ack(false)
		}
	}()
	return nil
}

func (b *Broker) Close() error {
	if b.ch != nil {
		_ = b.ch.Close()
	}
	return b.conn.Close()
}
