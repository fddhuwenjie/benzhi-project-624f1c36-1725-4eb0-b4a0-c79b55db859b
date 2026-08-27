package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"
)

func BuildEvent(sequence int64, caseID, eventType, actor string, revision int64, data any, previous string, now time.Time) (Event, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return Event{}, err
	}
	e := Event{Sequence: sequence, CaseID: caseID, Type: eventType, ActorID: actor, OccurredAt: now.UTC(), Revision: revision, Data: payload, PreviousDigest: previous}
	e.Digest = EventDigest(e)
	return e, nil
}

func EventDigest(e Event) string {
	h := sha256.New()
	parts := []string{strconv.FormatInt(e.Sequence, 10), e.CaseID, e.Type, e.ActorID, e.OccurredAt.UTC().Format(time.RFC3339Nano), strconv.FormatInt(e.Revision, 10), e.PreviousDigest}
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	h.Write(e.Data)
	return hex.EncodeToString(h.Sum(nil))
}

func VerifyEventChain(events []Event) error {
	previous := ""
	for i, e := range events {
		if e.Sequence != int64(i+1) {
			return NewError(CodeIntegrity, "事件序号在第 %d 项不连续", i+1)
		}
		if e.PreviousDigest != previous {
			return NewError(CodeIntegrity, "事件前序摘要在第 %d 项不匹配", i+1)
		}
		if EventDigest(e) != e.Digest {
			return NewError(CodeIntegrity, "事件摘要在第 %d 项无效", i+1)
		}
		previous = e.Digest
	}
	return nil
}
