package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"mastergate/internal/domain"
	"mastergate/internal/store"
)

type Service struct {
	repo               *store.Repository
	now                func() time.Time
	correctionDeadline time.Duration
	retestDeadline     time.Duration
	integrityCache     sync.Map
}

func New(repo *store.Repository) *Service {
	return NewWithDeadlines(repo, 24*time.Hour, 48*time.Hour)
}

func NewWithDeadlines(repo *store.Repository, correctionDeadline, retestDeadline time.Duration) *Service {
	if correctionDeadline <= 0 {
		correctionDeadline = 24 * time.Hour
	}
	if retestDeadline <= 0 {
		retestDeadline = 48 * time.Hour
	}
	return &Service{
		repo:               repo,
		now:                time.Now,
		correctionDeadline: correctionDeadline,
		retestDeadline:     retestDeadline,
	}
}

func fingerprint(command any) (string, error) {
	b, err := json.Marshal(command)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Service) execute(ctx context.Context, meta Metadata, caseID, actor, event string, create bool, status int, command any, fn func(*store.CaseData) (any, error)) (CommandResult, error) {
	return s.executeMutation(ctx, meta, store.Mutation{CaseID: caseID, ActorID: actor, EventType: event, Create: create, StatusCode: status}, command, fn)
}

func (s *Service) executeMutation(ctx context.Context, meta Metadata, mutation store.Mutation, command any, fn func(*store.CaseData) (any, error)) (CommandResult, error) {
	fp, err := fingerprint(command)
	if err != nil {
		return CommandResult{}, err
	}
	mutation.RequestID = meta.RequestID
	mutation.Fingerprint = fp
	mutation.ExpectedRevision = meta.ExpectedRevision
	r, err := s.repo.Execute(ctx, mutation, fn)
	if err != nil {
		return CommandResult{}, err
	}
	var result CommandResult
	if err := json.Unmarshal(r.Response, &result); err != nil {
		return CommandResult{}, err
	}
	return result, nil
}

func commandResult(data *store.CaseData) CommandResult {
	return CommandResult{Case: data.Case, Manifest: data.Manifest}
}

func requireEngineer(data *store.CaseData, actor string) error {
	if err := domain.ValidateIdentifier("操作人", actor); err != nil {
		return err
	}
	if actor != data.Case.EngineerID {
		return domain.NewError(domain.CodeForbidden, "仅案件工程师可提交业务证据")
	}
	return nil
}
