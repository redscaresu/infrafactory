package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// readSSM reads, for each public parameter (awsPublicParameterPrefix) a
// replayed GetParameter answered with an AMI id (awsAMIIDRe), in order of
// first mention, that image: one DescribeImages ImageId.1=<id> each, sent
// through r.followUp as readEC2's follow-ups are. A loaded copy launches
// from a parameter's AMI only when the read holds the image. A replay
// that read no such parameter adds nothing; an OK GetParameter whose
// answer names no parameter fails the read.
func (r *awsReplay) readSSM(ctx context.Context, replayed []AWSReadExchange) ([]AWSReadExchange, error) {
	var images []string
	for _, exchange := range replayed {
		if exchange.Service != "ssm" || exchange.Action != "GetParameter" || !awsAnswerOK(exchange.Status) {
			continue
		}
		var answer ssmParameterAnswer
		if err := json.Unmarshal([]byte(exchange.Body), &answer); err != nil {
			return nil, fmt.Errorf("finding the image GetParameter %s read: the answer does not parse: %w", exchange.Params, err)
		}
		if strings.TrimSpace(answer.Parameter.Name) == "" {
			return nil, fmt.Errorf("finding the image GetParameter %s read: the answer names no parameter", exchange.Params)
		}
		if strings.HasPrefix(answer.Parameter.Name, awsPublicParameterPrefix) && awsAMIIDRe.MatchString(answer.Parameter.Value) {
			images = awsAppendNew(images, answer.Parameter.Value)
		}
	}
	var out []AWSReadExchange
	for _, image := range images {
		pages, err := r.followUp(ctx, "ec2", "DescribeImages", map[string]string{"ImageId.1": image})
		if err != nil {
			return nil, fmt.Errorf("image %q: %w", image, err)
		}
		out = append(out, pages...)
	}
	return out, nil
}

// ssmParameterAnswer is the part of a GetParameter answer readSSM reads.
type ssmParameterAnswer struct {
	Parameter struct {
		Name, Value string
	}
}
