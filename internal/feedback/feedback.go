package feedback

type Failure struct {
	Layer    string
	Stage    string
	Status   string
	Check    string
	Policy   string
	Command  string
	Detail   string
	Stdout   string
	Stderr   string
	Resource string
	// Origin is the layer the failure came from, and OriginStage the
	// stage; see cli.FailureSummary.
	Origin      string
	OriginStage string
}
