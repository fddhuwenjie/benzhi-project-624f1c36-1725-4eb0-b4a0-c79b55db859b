package store

import (
	"context"
	"mastergate/internal/domain"
	"testing"
)

func TestExecuteIdempotencyAndConflict(t *testing.T) {
	r, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first := Mutation{RequestID: "req01", Fingerprint: "fp1", CaseID: "case01", Create: true, ActorID: "eng01", EventType: "created"}
	value := func(data *CaseData) (any, error) {
		data.Case = domain.DeliveryCase{CaseID: "case01", Revision: 1}
		return map[string]string{"ok": "yes"}, nil
	}
	one, err := r.Execute(ctx, first, value)
	if err != nil {
		t.Fatal(err)
	}
	two, err := r.Execute(ctx, first, value)
	if err != nil || !two.Replayed || string(one.Response) != string(two.Response) {
		t.Fatalf("幂等重放失败: %#v %v", two, err)
	}
	first.Fingerprint = "different"
	if _, err := r.Execute(ctx, first, value); domain.ErrorCodeOf(err) != domain.CodeIdempotent {
		t.Fatalf("应拒绝不同指纹重放: %v", err)
	}
	next := Mutation{RequestID: "req02", Fingerprint: "fp2", CaseID: "case01", ExpectedRevision: 0, ActorID: "eng01", EventType: "change"}
	if _, err := r.Execute(ctx, next, func(data *CaseData) (any, error) { return nil, nil }); domain.ErrorCodeOf(err) != domain.CodeConflict {
		t.Fatalf("应报告修订冲突: %v", err)
	}
}
