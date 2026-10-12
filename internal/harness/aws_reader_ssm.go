package harness

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// readSSM reads, for each public parameter (awsPublicParameterPrefix) a
// replayed GetParameter answered with an AMI id (awsAMIIDRe), in order of
// first mention, that image: one DescribeImages ImageId.1=<id> each, sent
// through r.followUp as readEC2's follow-ups are. A loaded copy launches
// from a parameter's AMI only when the read holds the image, so an image
// that no OK page holds (an empty imagesSet, InvalidAMIID.NotFound) fails
// the read, as does an OK GetParameter whose answer names no parameter.
// A replay that read no such parameter adds nothing.
func (r *awsReplay) readSSM(ctx context.Context, replayed []AWSReadExchange) ([]AWSReadExchange, error) {
	var images []string
	parameters := map[string]string{} // the first parameter naming each image
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
		if awsPublicParameter(answer.Parameter.Name) && awsAMIIDRe.MatchString(answer.Parameter.Value) {
			if _, seen := parameters[answer.Parameter.Value]; !seen {
				images = append(images, answer.Parameter.Value)
				parameters[answer.Parameter.Value] = answer.Parameter.Name
			}
		}
	}
	var out []AWSReadExchange
	for _, image := range images {
		pages, err := r.followUp(ctx, "ec2", "DescribeImages", map[string]string{"ImageId.1": image})
		if err == nil {
			err = awsCheckImageRead(pages, image)
		}
		if err != nil {
			return nil, fmt.Errorf("image %q of parameter %q: %w", image, parameters[image], err)
		}
		out = append(out, pages...)
	}
	return out, nil
}

// awsCheckImageRead passes pages when an OK one holds image in its
// imagesSet.
func awsCheckImageRead(pages []AWSReadExchange, image string) error {
	for _, page := range pages {
		if !awsAnswerOK(page.Status) {
			continue
		}
		var answer ec2ImagesAnswer
		if err := xml.Unmarshal([]byte(page.Body), &answer); err != nil {
			return fmt.Errorf("DescribeImages %s: the answer does not parse: %w", page.Params, err)
		}
		if slices.Contains(answer.IDs, image) {
			return nil
		}
	}
	return errors.New("DescribeImages answered without the image")
}

// ec2ImagesAnswer is the part of a DescribeImages answer readSSM reads.
type ec2ImagesAnswer struct {
	IDs []string `xml:"imagesSet>item>imageId"`
}

// ssmParameterAnswer is the part of a GetParameter answer readSSM reads.
type ssmParameterAnswer struct {
	Parameter struct {
		Name, Value string
	}
}
