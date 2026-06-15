package raft

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
	hraft "github.com/hashicorp/raft"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// Store is the Raft-backed distributed state store for Gatekeeper.
// It implements the contracts.RaftStore interface.
type Store struct {
	mu     sync.RWMutex
	raft   *hraft.Raft
	fsm    *FSM
	leader bool
}

// NewStore creates a new Raft store.
func NewStore(r *hraft.Raft, fsm *FSM) *Store {
	return &Store{
		raft: r,
		fsm:  fsm,
	}
}

// Apply submits a command to the Raft log for consensus.
func (s *Store) Apply(cmd RaftCommand) error {
	data, err := encodeCommand(cmd.Type, cmd.Payload)
	if err != nil {
		return fmt.Errorf("store apply encode: %w", err)
	}

	future := s.raft.Apply(data, 0)
	if err := future.Error(); err != nil {
		return fmt.Errorf("store apply: %w", err)
	}

	if err, ok := future.Response().(error); ok && err != nil {
		return fmt.Errorf("store apply fsm: %w", err)
	}

	return nil
}

// IsLeader returns true if this node is the Raft leader.
func (s *Store) IsLeader() bool {
	return s.raft.State() == hraft.Leader
}

// LeaderAddr returns the address of the current Raft leader.
func (s *Store) LeaderAddr() string {
	return string(s.raft.Leader())
}

// ReadFactor returns the in-memory factor state (may lag by one Raft tick).
func (s *Store) ReadFactor(id uuid.UUID) (*models.Factor, bool) {
	return s.fsm.ReadFactor(id)
}

// ReadChallenge returns the in-memory challenge state.
func (s *Store) ReadChallenge(id uuid.UUID) (*models.Challenge, bool) {
	return s.fsm.ReadChallenge(id)
}

// RaftCommand is the typed command for the Raft store.
type RaftCommand struct {
	Type    models.RaftCommandType
	Payload interface{}
}
