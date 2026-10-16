package dao

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type VolunteerSupportSession struct {
	UserID        int64
	State         string
	Problem       string
	Category      string
	Environment   string
	LastMessageID int32
	LastReply     string
	TicketID      int64
	SubmittedAt   *time.Time
}

type VolunteerSupportTicket struct {
	ID          int64
	UserID      int64
	Status      string
	Problem     string
	Category    string
	Environment string
	AssignedTo  int64
	ClaimedAt   *time.Time
	ResolvedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (d *Dao) LoadVolunteerSupportSession(ctx context.Context, userID int64) (*VolunteerSupportSession, error) {
	if d == nil || d.Postgres == nil || d.Pool == nil {
		return nil, errors.New("sync: Volunteer Support session store is not configured")
	}
	session := &VolunteerSupportSession{UserID: userID, State: "idle"}
	err := d.Pool.QueryRow(ctx, `SELECT state,problem,category,environment,last_message_id,last_reply,COALESCE(ticket_id,0),submitted_at
FROM volunteer_support_sessions WHERE user_id=$1`, userID).Scan(
		&session.State, &session.Problem, &session.Category, &session.Environment,
		&session.LastMessageID, &session.LastReply, &session.TicketID, &session.SubmittedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return session, nil
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (d *Dao) SaveVolunteerSupportSession(ctx context.Context, session *VolunteerSupportSession) error {
	if d == nil || d.Postgres == nil || d.Pool == nil {
		return errors.New("sync: Volunteer Support session store is not configured")
	}
	if session == nil || session.UserID <= 0 || session.State == "" {
		return errors.New("sync: invalid Volunteer Support session")
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, session.UserID); err != nil {
		return err
	}
	if session.State == "submitted" && session.SubmittedAt != nil && session.TicketID == 0 {
		var ticketID int64
		err = tx.QueryRow(ctx, `SELECT COALESCE(ticket_id,0) FROM volunteer_support_sessions WHERE user_id=$1 FOR UPDATE`, session.UserID).Scan(&ticketID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		}
		if err != nil {
			return err
		}
		if ticketID == 0 {
			err = tx.QueryRow(ctx, `INSERT INTO volunteer_support_tickets
	(user_id,status,problem,category,environment)
VALUES($1,'queued',$2,$3,$4)
RETURNING id`, session.UserID, session.Problem, session.Category, session.Environment).Scan(&session.TicketID)
			if err != nil {
				return err
			}
		} else {
			session.TicketID = ticketID
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO volunteer_support_sessions
	(user_id,state,problem,category,environment,last_message_id,last_reply,ticket_id,submitted_at)
VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,0),$9)
ON CONFLICT (user_id) DO UPDATE SET
 state=EXCLUDED.state,
 problem=EXCLUDED.problem,
 category=EXCLUDED.category,
 environment=EXCLUDED.environment,
 last_message_id=EXCLUDED.last_message_id,
 last_reply=EXCLUDED.last_reply,
 ticket_id=EXCLUDED.ticket_id,
 submitted_at=EXCLUDED.submitted_at,
 updated_at=CURRENT_TIMESTAMP`, session.UserID, session.State, session.Problem,
		session.Category, session.Environment, session.LastMessageID, session.LastReply, session.TicketID, session.SubmittedAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *Dao) ClaimNextVolunteerSupportTicket(ctx context.Context, volunteerID int64) (*VolunteerSupportTicket, error) {
	if d == nil || d.Postgres == nil || d.Pool == nil {
		return nil, errors.New("sync: Volunteer Support ticket store is not configured")
	}
	if volunteerID <= 0 {
		return nil, errors.New("sync: invalid Volunteer Support volunteer")
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ticket := &VolunteerSupportTicket{}
	err = tx.QueryRow(ctx, `SELECT id,user_id,status,problem,category,environment,
COALESCE(assigned_to,0),claimed_at,resolved_at,created_at,updated_at
FROM volunteer_support_tickets
WHERE status='queued'
ORDER BY created_at,id
FOR UPDATE SKIP LOCKED
LIMIT 1`).Scan(
		&ticket.ID, &ticket.UserID, &ticket.Status, &ticket.Problem, &ticket.Category,
		&ticket.Environment, &ticket.AssignedTo, &ticket.ClaimedAt, &ticket.ResolvedAt,
		&ticket.CreatedAt, &ticket.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := time.Now()
	err = tx.QueryRow(ctx, `UPDATE volunteer_support_tickets
SET status='assigned', assigned_to=$1, claimed_at=$2, updated_at=CURRENT_TIMESTAMP
WHERE id=$3
RETURNING id,user_id,status,problem,category,environment,
COALESCE(assigned_to,0),claimed_at,resolved_at,created_at,updated_at`, volunteerID, now, ticket.ID).Scan(
		&ticket.ID, &ticket.UserID, &ticket.Status, &ticket.Problem, &ticket.Category,
		&ticket.Environment, &ticket.AssignedTo, &ticket.ClaimedAt, &ticket.ResolvedAt,
		&ticket.CreatedAt, &ticket.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return ticket, nil
}
