package store

import (
	"context"
	"encoding/json"
	"sort"

	"mastergate/internal/domain"
)

func (r *Repository) GetCase(ctx context.Context, caseID string) (CaseData, error) {
	if err := ctx.Err(); err != nil {
		return CaseData{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	data, ok := r.data.Cases[caseID]
	if !ok {
		return CaseData{}, domain.NewError(domain.CodeNotFound, "案件不存在")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return CaseData{}, err
	}
	var result CaseData
	if err := json.Unmarshal(b, &result); err != nil {
		return CaseData{}, err
	}
	return result, nil
}

func (r *Repository) ListCases(ctx context.Context) ([]domain.DeliveryCase, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]domain.DeliveryCase, 0, len(r.data.Cases))
	for _, data := range r.data.Cases {
		result = append(result, data.Case)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (r *Repository) Verify(ctx context.Context, caseID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	data, ok := r.data.Cases[caseID]
	if !ok {
		return domain.NewError(domain.CodeNotFound, "案件不存在")
	}
	return validateCase(data)
}

func (r *Repository) VerifyAll(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return validateSnapshot(r.data)
}
