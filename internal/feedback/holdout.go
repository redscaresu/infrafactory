package feedback

func HoldoutFeedback(criteriaOnly bool, failures []Failure) []Failure {
	if criteriaOnly {
		return nil
	}
	return failures
}
