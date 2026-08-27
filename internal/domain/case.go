package domain

import "time"

func NewDeliveryCase(id, program, version, engineer, masterSHA string, profile StandardProfile, now time.Time) (*DeliveryCase, error) {
	for label, value := range map[string]string{"案件标识": id, "节目编号": program, "制作版本": version, "工程师": engineer} {
		if err := ValidateIdentifier(label, value); err != nil {
			return nil, err
		}
	}
	if err := ValidateSHA256("母版摘要", masterSHA); err != nil {
		return nil, err
	}
	if err := ValidateStandard(profile); err != nil {
		return nil, err
	}
	return &DeliveryCase{CaseID: id, ProgramCode: program, MasterVersion: version, EngineerID: engineer, MasterSHA256: masterSHA, StandardProfile: profile, State: StateDraft, Revision: 1, CreatedAt: now.UTC()}, nil
}

func (c *DeliveryCase) EnsureMutable() error {
	if c.State.Terminal() {
		return NewError(CodeState, "终态案件禁止修改业务证据")
	}
	return nil
}

func (c *DeliveryCase) ReviseMetadata(program, version, engineer, masterSHA string, profile StandardProfile) error {
	if c.State != StateDraft {
		return NewError(CodeState, "仅草稿案件可修订元数据")
	}
	for label, value := range map[string]string{"节目编号": program, "制作版本": version, "工程师": engineer} {
		if err := ValidateIdentifier(label, value); err != nil {
			return err
		}
	}
	if err := ValidateSHA256("母版摘要", masterSHA); err != nil {
		return err
	}
	if err := ValidateStandard(profile); err != nil {
		return err
	}
	c.ProgramCode = program
	c.MasterVersion = version
	c.EngineerID = engineer
	c.MasterSHA256 = masterSHA
	c.StandardProfile = profile
	c.Revision++
	return nil
}

func (c *DeliveryCase) FreezeBaseline(now time.Time) error {
	if err := c.EnsureMutable(); err != nil {
		return err
	}
	if c.State != StateDraft {
		return NewError(CodeState, "仅草稿案件可冻结基线")
	}
	t := now.UTC()
	c.BaselineFrozenAt = &t
	c.State = StateBaseline
	c.Revision++
	return nil
}

func (c *DeliveryCase) MarkMeasuring() error {
	if err := c.EnsureMutable(); err != nil {
		return err
	}
	if c.State != StateBaseline && c.State != StateMeasuring {
		return NewError(CodeState, "当前状态不能登记测量证据")
	}
	c.State = StateMeasuring
	c.Revision++
	return nil
}

func (c *DeliveryCase) MarkEvaluation(failed bool) error {
	if err := c.EnsureMutable(); err != nil {
		return err
	}
	if c.State != StateMeasuring && c.State != StateRemediation {
		return NewError(CodeState, "当前状态不能执行规则判定")
	}
	if failed {
		c.State = StateRemediation
	} else {
		c.State = StateReview
	}
	c.Revision++
	return nil
}

func (c *DeliveryCase) MarkRemediation() error {
	if err := c.EnsureMutable(); err != nil {
		return err
	}
	if c.State != StateRemediation {
		return NewError(CodeState, "仅整改状态可登记偏差处理")
	}
	c.Revision++
	return nil
}

func (c *DeliveryCase) MarkReadyForReview() error {
	if err := c.EnsureMutable(); err != nil {
		return err
	}
	if c.State != StateRemediation {
		return NewError(CodeState, "案件不在整改状态")
	}
	c.State = StateReview
	c.Revision++
	return nil
}

func (c *DeliveryCase) Approve(reviewer string, annotations []ReviewAnnotation) error {
	if err := c.EnsureMutable(); err != nil {
		return err
	}
	if c.State != StateReview {
		return NewError(CodeState, "仅待复核案件可批准")
	}
	if reviewer == c.EngineerID {
		return NewError(CodeForbidden, "复核员必须与提交工程师不同")
	}
	if err := ValidateIdentifier("复核员", reviewer); err != nil {
		return err
	}
	c.ReviewedBy = reviewer
	c.setReviewAnnotations(annotations)
	c.State = StateApproved
	c.Revision++
	return nil
}

func (c *DeliveryCase) Reject(reviewer, code, detail string, annotations []ReviewAnnotation) error {
	if err := c.EnsureMutable(); err != nil {
		return err
	}
	if c.State != StateReview {
		return NewError(CodeState, "仅待复核案件可拒绝")
	}
	if reviewer == c.EngineerID {
		return NewError(CodeForbidden, "复核员必须与提交工程师不同")
	}
	if err := ValidateIdentifier("复核员", reviewer); err != nil {
		return err
	}
	if err := ValidateIdentifier("拒绝代码", code); err != nil {
		return err
	}
	if err := ValidateText("拒绝说明", detail, 500); err != nil {
		return err
	}
	c.ReviewedBy = reviewer
	c.setReviewAnnotations(annotations)
	c.RejectionCode = code
	c.RejectionDetail = detail
	c.State = StateRejected
	c.Revision++
	return nil
}

func (c *DeliveryCase) setReviewAnnotations(annotations []ReviewAnnotation) {
	c.ReviewAnnotations = append([]ReviewAnnotation(nil), annotations...)
	c.ReviewChecks = make([]string, 0, len(annotations))
	for _, annotation := range annotations {
		c.ReviewChecks = append(c.ReviewChecks, annotation.CheckType)
	}
}
