package events

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const socketTicketTTL = 60 * time.Second

type Broker struct {
	pool *pgxpool.Pool
	mu sync.RWMutex
	subs map[string]map[chan int64]struct{}
}

type notificationPayload struct {
	UserID string `json:"user_id"`
	Sequence int64 `json:"sequence"`
}

func NewBroker(pool *pgxpool.Pool) *Broker {
	return &Broker{pool:pool,subs:make(map[string]map[chan int64]struct{})}
}

func (b *Broker) Start(ctx context.Context) { go b.listen(ctx) }

func (b *Broker) listen(ctx context.Context) {
	for {
		if ctx.Err()!=nil { return }
		conn,err:=b.pool.Acquire(ctx)
		if err!=nil { time.Sleep(time.Second); continue }
		_,err=conn.Exec(ctx,"LISTEN clusterstor_account_events")
		if err!=nil { conn.Release(); time.Sleep(time.Second); continue }
		for {
			n,err:=conn.Conn().WaitForNotification(ctx)
			if err!=nil { break }
			var payload notificationPayload
			if json.Unmarshal([]byte(n.Payload),&payload)!=nil || payload.UserID=="" { continue }
			b.publish(payload.UserID,payload.Sequence)
		}
		conn.Release()
	}
}

func (b *Broker) publish(userID string, sequence int64) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch:=range b.subs[userID] {
		select { case ch<-sequence: default: }
	}
}

func (b *Broker) Subscribe(userID string) (<-chan int64, func()) {
	ch:=make(chan int64,8)
	b.mu.Lock()
	if b.subs[userID]==nil { b.subs[userID]=make(map[chan int64]struct{}) }
	b.subs[userID][ch]=struct{}{}
	b.mu.Unlock()
	return ch,func(){
		b.mu.Lock()
		if subs:=b.subs[userID]; subs!=nil { delete(subs,ch); if len(subs)==0 { delete(b.subs,userID) } }
		b.mu.Unlock()
		close(ch)
	}
}

func (s *Service) CreateSocketTicket(ctx context.Context,userID string) (string,time.Time,error) {
	raw:=make([]byte,32)
	if _,err:=rand.Read(raw); err!=nil { return "",time.Time{},fmt.Errorf("generate socket ticket: %w",err) }
	ticket:=base64.RawURLEncoding.EncodeToString(raw)
	hash:=sha256.Sum256([]byte(ticket))
	expires:=time.Now().UTC().Add(socketTicketTTL)
	_,err:=s.pool.Exec(ctx,"INSERT INTO event_socket_tickets(user_id,ticket_hash,expires_at) VALUES ($1::uuid,$2,$3)",userID,hash[:],expires)
	if err!=nil { return "",time.Time{},fmt.Errorf("store socket ticket: %w",err) }
	return ticket,expires,nil
}

func (s *Service) ConsumeSocketTicket(ctx context.Context,ticket string) (string,error) {
	if ticket=="" { return "",errors.New("invalid socket ticket") }
	hash:=sha256.Sum256([]byte(ticket))
	tx,err:=s.pool.Begin(ctx)
	if err!=nil { return "",err }
	defer tx.Rollback(ctx)
	var userID string
	err=tx.QueryRow(ctx,"SELECT user_id::text FROM event_socket_tickets WHERE ticket_hash=$1 AND consumed_at IS NULL AND expires_at>now() FOR UPDATE",hash[:]).Scan(&userID)
	if errors.Is(err,pgx.ErrNoRows) { return "",errors.New("invalid socket ticket") }
	if err!=nil { return "",err }
	if _,err=tx.Exec(ctx,"UPDATE event_socket_tickets SET consumed_at=now() WHERE ticket_hash=$1",hash[:]); err!=nil { return "",err }
	if err=tx.Commit(ctx); err!=nil { return "",err }
	return userID,nil
}
