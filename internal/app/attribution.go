package app

// Actor is caller-declared provenance, not identity verification or authority.
// Omitted actors retain the user attribution of existing owner-facing commands.
func ownerMutationActor(actor string) (string, error) {
	if actor == "" {
		return "user", nil
	}
	if actor != "user" && actor != "agent" {
		return "", Invalid("actor must be user or agent")
	}
	return actor, nil
}

// Adding attribution to an exact legacy command may replay its old receipt,
// without reapplying the effect or rewriting ambiguous historical attribution.
// Explicitly attributed receipts still conflict when their actor is changed.
type attributedCommand interface {
	withoutActor() (any, bool)
}

func (input ApplyItemAction) withoutActor() (any, bool) {
	present := input.Actor != ""
	input.Actor = ""
	return input, present
}

func (input SetUserNote) withoutActor() (any, bool) {
	present := input.Actor != ""
	input.Actor = ""
	return input, present
}

func (input PutItem) withoutActor() (any, bool) {
	present := input.Actor != ""
	input.Actor = ""
	return input, present
}
