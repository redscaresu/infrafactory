package generator

import (
	"context"
	"fmt"
)

// defaultSeedGenerator provides deterministic runtime behavior until
// concrete transport-backed generators are wired.
type defaultSeedGenerator struct {
	agentType string
}

func NewDefaultSeedGenerator(agentType string) SeedGenerator {
	return &defaultSeedGenerator{agentType: agentType}
}

func (g *defaultSeedGenerator) Generate(_ context.Context, _ Request) (*GeneratedCode, error) {
	return nil, NewGenerateError(
		ErrTransportFailed,
		"default_generator",
		fmt.Errorf("default seed generator for agent type %q is not implemented", g.agentType),
	)
}
