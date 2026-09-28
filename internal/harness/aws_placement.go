package harness

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// AWSChildScopedTypes carry neither an arn nor an owner_id because they
// live inside a parent resource; the value names the attributes that
// reference the parent. As with ChildScopedTypes, a child is placed only
// through those parents.
var AWSChildScopedTypes = map[string][]string{
	"aws_route":                   {"route_table_id"},
	"aws_route_table_association": {"route_table_id", "subnet_id"},
}

// UnplacedAWSResources names every managed resource in workDir's live
// state that it cannot place in AWS account accountID. An allowlist, like
// unplacedResources: a resource is placed when it carries an arn or an
// owner_id and every one it carries names accountID; an
// AWSChildScopedTypes child when every parent it names is a managed
// resource in the same state. Everything else is refused, including an
// arn with an empty account whatever owner_id says, which is what a
// provider that skipped the account lookup builds client-side.
func UnplacedAWSResources(workDir, accountID string) ([]string, error) {
	if strings.TrimSpace(accountID) == "" {
		return nil, errors.New("no AWS account id to place the state in")
	}
	state, err := loadLiveTerraformState(filepath.Join(workDir, LiveStateFilename))
	if err != nil {
		return nil, err
	}

	// Parents count only if this state manages them: a data source's id
	// names something the run looked up, not something it owns.
	ids := map[string]bool{}
	for _, resource := range state.Resources {
		if !managedCloudResource(resource) {
			continue
		}
		for _, instance := range resource.Instances {
			if id, _ := instance.Attributes["id"].(string); id != "" {
				ids[id] = true
			}
		}
	}

	var unplaced []string
	for _, resource := range state.Resources {
		if !managedCloudResource(resource) {
			continue
		}
		for _, instance := range resource.Instances {
			id, _ := instance.Attributes["id"].(string)
			name := fmt.Sprintf("%s (%s)", resource.Type, id)
			if parents, child := AWSChildScopedTypes[resource.Type]; child {
				unplaced = append(unplaced, unplacedAWSChild(name, instance.Attributes, parents, ids)...)
			} else if why := awsAccountRefusal(instance.Attributes, accountID); why != "" {
				unplaced = append(unplaced, name+" "+why)
			}
		}
	}
	return unplaced, nil
}

func unplacedAWSChild(name string, attrs map[string]any, parents []string, ids map[string]bool) []string {
	var unplaced []string
	for _, attr := range parents {
		if parent, _ := attrs[attr].(string); !ids[parent] {
			unplaced = append(unplaced, fmt.Sprintf("%s whose %s %q is not in the state", name, attr, parent))
		}
	}
	return unplaced
}

// awsAccountRefusal says why attrs cannot be placed in accountID, or
// returns "" when they can.
func awsAccountRefusal(attrs map[string]any, accountID string) string {
	var accounts []string
	if arn, _ := attrs["arn"].(string); arn != "" {
		// arn:partition:service:region:account:resource
		fields := strings.SplitN(arn, ":", 6)
		if len(fields) != 6 {
			return fmt.Sprintf("whose arn %q is not an ARN", arn)
		}
		if fields[4] == "" {
			return "whose arn has no account"
		}
		accounts = append(accounts, fields[4])
	}
	if owner, _ := attrs["owner_id"].(string); owner != "" {
		accounts = append(accounts, owner)
	}
	if len(accounts) == 0 {
		return "carries no account id"
	}
	for _, account := range accounts {
		if account != accountID {
			return fmt.Sprintf("in account %q", account)
		}
	}
	return ""
}
