package generator

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

var (
	ErrGenerateFailed     = errors.New("generate failed")
	ErrPromptRenderFailed = errors.New("prompt render failed")
	ErrTransportFailed    = errors.New("generator transport failed")
	ErrParseFailed        = errors.New("generator parse failed")
)

type SeedGenerator interface {
	Generate(context.Context, Request) (*GeneratedCode, error)
}

type SeedGeneratorFunc func(context.Context, Request) (*GeneratedCode, error)

func (f SeedGeneratorFunc) Generate(ctx context.Context, req Request) (*GeneratedCode, error) {
	return f(ctx, req)
}

type Request struct {
	ScenarioPath       string
	ScenarioYAML       []byte
	FeedbackJSON       []byte
	Iteration          int
	ProviderSchemaJSON []byte
	Layer3Enabled      bool
	Cloud              string
	// UserDataLine is set only for an AWS scenario with a service: block,
	// whose boot script infrafactory renders itself (AWSUserDataFile).
	UserDataLine string
	// AMIID is set only for AWS: the AMI id every aws_instance is told to
	// write verbatim, since phase 1 forbids an AMI lookup.
	AMIID string
}

// AWSUserDataFile is the boot script infrafactory renders from an AWS
// scenario's service: block into the generated directory. The model never
// writes it: it only references it, with exactly AWSUserDataLine.
const AWSUserDataFile = "infrafactory-user-data.sh"

// AWSUserDataLine is the one line the model writes on the instance that
// runs the service.
const AWSUserDataLine = `user_data = file("${path.module}/` + AWSUserDataFile + `")`

type GeneratedCode struct {
	Files    map[string][]byte
	Metadata GenerationMetadata
}

type GenerationMetadata struct {
	Generator string
	Phases    []PhaseResult
}

type PhaseResult struct {
	Name   string
	Prompt []byte
	Output []byte
}

func (c *GeneratedCode) Validate() error {
	if c == nil {
		return fmt.Errorf("%w: generated code is nil", ErrParseFailed)
	}
	if len(c.Files) == 0 {
		return fmt.Errorf("%w: no files generated", ErrParseFailed)
	}
	for name, content := range c.Files {
		if name == "" {
			return fmt.Errorf("%w: generated file name is empty", ErrParseFailed)
		}
		if len(content) == 0 {
			return fmt.Errorf("%w: generated file %q is empty", ErrParseFailed, name)
		}
	}
	return nil
}

func (c *GeneratedCode) SortedFileNames() []string {
	if c == nil {
		return nil
	}

	names := make([]string, 0, len(c.Files))
	for name := range c.Files {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

type GenerateError struct {
	Kind  error
	Phase string
	Err   error
}

func NewGenerateError(kind error, phase string, err error) *GenerateError {
	if kind == nil {
		kind = ErrGenerateFailed
	}

	return &GenerateError{
		Kind:  kind,
		Phase: phase,
		Err:   err,
	}
}

func (e *GenerateError) Error() string {
	if e == nil {
		return ErrGenerateFailed.Error()
	}

	if e.Phase == "" {
		return fmt.Sprintf("%s: %v", e.Kind, e.Err)
	}

	return fmt.Sprintf("%s: phase %q: %v", e.Kind, e.Phase, e.Err)
}

func (e *GenerateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *GenerateError) Is(target error) bool {
	if target == ErrGenerateFailed {
		return true
	}
	return e.Kind == target
}
