package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// The attributes the EC2 follow-ups read: both DNS attributes of each
// VPC, and each DescribeInstanceAttribute that terraform-provider-aws
// 5.100.0 sends to read an aws_instance (internal/service/ec2/
// ec2_instance.go, resourceInstanceRead and readInstanceShutdownBehavior).
var (
	awsReadVPCAttributes      = []string{"enableDnsSupport", "enableDnsHostnames"}
	awsReadInstanceAttributes = []string{"instanceInitiatedShutdownBehavior", "disableApiStop", "disableApiTermination", "userData"}
)

// readEC2 reads what the replay touched on EC2: for each VPC a replayed
// EC2 answer lists a resource in (awsListedVPCIDs), in order of first
// mention, its surroundings and follow-ups (readEC2VPC), then the AZs
// once, unless the replay read them unfiltered. Each request is an ADR-0041 entry sent through
// r.pages, so it is walled, signed, paged, throttled and recorded as a
// replayed entry is. A replay that touched no VPC adds nothing.
func (r *awsReplay) readEC2(ctx context.Context, replayed []AWSReadExchange) ([]AWSReadExchange, error) {
	var vpcs []string
	for _, exchange := range replayed {
		if exchange.Service != "ec2" || !awsAnswerOK(exchange.Status) {
			continue
		}
		listed, err := awsListedVPCIDs([]byte(exchange.Body))
		if err != nil {
			return nil, fmt.Errorf("finding the VPCs %s %s touched: %w", exchange.Action, exchange.Params, err)
		}
		vpcs = awsAppendNew(vpcs, listed...)
	}
	if len(vpcs) == 0 {
		return nil, nil
	}
	var out []AWSReadExchange
	for _, vpc := range vpcs {
		exchanges, err := r.readEC2VPC(ctx, vpc)
		if err != nil {
			return nil, err
		}
		out = append(out, exchanges...)
	}
	if slices.ContainsFunc(replayed, awsReadAllZones) {
		return out, nil
	}
	zones, err := r.followUp(ctx, "ec2", "DescribeAvailabilityZones", map[string]string{})
	return append(out, zones...), err
}

// awsListedVPCIDs is the VPC of each resource an EC2 answer lists, read
// only from the resource's own field: root>list>item>vpcId, and an
// instance's root>reservationSet>item>instancesSet>item>vpcId. Any other
// vpcId (a rule's peered group, a gateway's attachment) names a VPC the
// resource refers to, not one the read touched, and is dropped.
func awsListedVPCIDs(body []byte) ([]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	var path, out []string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			path = append(path, t.Name.Local)
			if !awsOwnVPCIDPath(path) {
				continue
			}
			var id string
			if err := dec.DecodeElement(&id, &t); err != nil {
				return nil, err
			}
			path = path[:len(path)-1]
			out = awsAppendNew(out, id)
		case xml.EndElement:
			path = path[:len(path)-1]
		}
	}
}

// awsOwnVPCIDPath reports whether path, from the answer's root, is a
// listed resource's own vpcId.
func awsOwnVPCIDPath(path []string) bool {
	switch {
	case len(path) == 4:
		return path[2] == "item" && path[3] == "vpcId"
	case len(path) == 6:
		return slices.Equal(path[1:], []string{"reservationSet", "item", "instancesSet", "item", "vpcId"})
	}
	return false
}

// readEC2VPC reads vpc's surroundings, each list filtered to vpc where
// the API takes a vpc-id filter, else to the ids the VPC's own lists
// name: SG rules by its SGs' group-id, addresses by its interfaces'
// network-interface-id, key pairs by its instances' key-name, volumes by
// its instances' attachment.instance-id (aws_instance's read reads its
// block devices' volumes). Then both VPC attributes, and each instance's
// attributes.
func (r *awsReplay) readEC2VPC(ctx context.Context, vpc string) ([]AWSReadExchange, error) {
	var out []AWSReadExchange
	read := func(action string, params map[string]string) ([]AWSReadExchange, error) {
		pages, err := r.followUp(ctx, "ec2", action, params)
		out = append(out, pages...)
		return pages, err
	}
	// readBy reads action filtered by name to values, and nothing when
	// there are none: an empty filter would read the whole region.
	readBy := func(action, name string, values []string) error {
		if len(values) == 0 {
			return nil
		}
		_, err := read(action, awsFilterParams(name, values...))
		return err
	}
	inVPC := awsFilterParams("vpc-id", vpc)

	if _, err := read("DescribeSubnets", inVPC); err != nil {
		return out, err
	}
	groups, err := read("DescribeSecurityGroups", inVPC)
	if err != nil {
		return out, err
	}
	groupIDs, err := awsPageValues(groups, func(a ec2GroupsAnswer) []string { return a.ids() })
	if err != nil {
		return out, err
	}
	if err := readBy("DescribeSecurityGroupRules", "group-id", groupIDs); err != nil {
		return out, err
	}
	if _, err := read("DescribeRouteTables", inVPC); err != nil {
		return out, err
	}
	if _, err := read("DescribeInternetGateways", awsFilterParams("attachment.vpc-id", vpc)); err != nil {
		return out, err
	}
	interfaces, err := read("DescribeNetworkInterfaces", inVPC)
	if err != nil {
		return out, err
	}
	interfaceIDs, err := awsPageValues(interfaces, func(a ec2InterfacesAnswer) []string { return a.ids() })
	if err != nil {
		return out, err
	}
	if err := readBy("DescribeAddresses", "network-interface-id", interfaceIDs); err != nil {
		return out, err
	}
	instances, err := read("DescribeInstances", inVPC)
	if err != nil {
		return out, err
	}
	instanceIDs, err := awsPageValues(instances, func(a ec2InstancesAnswer) []string { return a.ids() })
	if err != nil {
		return out, err
	}
	keyNames, err := awsPageValues(instances, func(a ec2InstancesAnswer) []string { return a.keyNames() })
	if err != nil {
		return out, err
	}
	if err := readBy("DescribeKeyPairs", "key-name", keyNames); err != nil {
		return out, err
	}
	if err := readBy("DescribeVolumes", "attachment.instance-id", instanceIDs); err != nil {
		return out, err
	}
	for _, attribute := range awsReadVPCAttributes {
		if _, err := read("DescribeVpcAttribute", map[string]string{"VpcId": vpc, "Attribute": attribute}); err != nil {
			return out, err
		}
	}
	for _, instance := range instanceIDs {
		for _, attribute := range awsReadInstanceAttributes {
			if _, err := read("DescribeInstanceAttribute", map[string]string{"InstanceId": instance, "Attribute": attribute}); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

// followUp reads one follow-up, a query action of service, to its last
// page; an error names the call and its params.
func (r *awsReplay) followUp(ctx context.Context, service, action string, params map[string]string) ([]AWSReadExchange, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	pages, err := r.pages(ctx, AWSReadEntry{Service: service, Protocol: awsProtocolQuery, Action: action, Params: raw})
	if err != nil {
		return nil, fmt.Errorf("follow-up %s %s: %w", action, raw, err)
	}
	return pages, nil
}

// awsReadAllZones reports whether exchange read every AZ: an unfiltered
// DescribeAvailabilityZones that answered.
func awsReadAllZones(exchange AWSReadExchange) bool {
	if exchange.Service != "ec2" || exchange.Action != "DescribeAvailabilityZones" || !awsAnswerOK(exchange.Status) {
		return false
	}
	form, err := awsQueryForm(exchange.Params)
	return err == nil && len(form) == 0
}

// awsFilterParams is one EC2 Filter.1 naming name, matching any of values.
func awsFilterParams(name string, values ...string) map[string]string {
	params := map[string]string{"Filter.1.Name": name}
	for i, value := range values {
		params[fmt.Sprintf("Filter.1.Value.%d", i+1)] = value
	}
	return params
}

// The parts of EC2 list answers a follow-up is filtered by, each read by
// its path from the answer's root, so an id nested deeper (a group an
// instance or a rule refers to) is not taken for a listed one.
type ec2GroupsAnswer struct {
	Groups []struct {
		ID string `xml:"groupId"`
	} `xml:"securityGroupInfo>item"`
}

func (a ec2GroupsAnswer) ids() []string {
	var out []string
	for _, g := range a.Groups {
		out = append(out, g.ID)
	}
	return out
}

type ec2InterfacesAnswer struct {
	Interfaces []struct {
		ID string `xml:"networkInterfaceId"`
	} `xml:"networkInterfaceSet>item"`
}

func (a ec2InterfacesAnswer) ids() []string {
	var out []string
	for _, i := range a.Interfaces {
		out = append(out, i.ID)
	}
	return out
}

type ec2InstancesAnswer struct {
	Reservations []struct {
		Instances []struct {
			ID      string `xml:"instanceId"`
			KeyName string `xml:"keyName"`
		} `xml:"instancesSet>item"`
	} `xml:"reservationSet>item"`
}

func (a ec2InstancesAnswer) ids() []string {
	var out []string
	for _, r := range a.Reservations {
		for _, i := range r.Instances {
			out = append(out, i.ID)
		}
	}
	return out
}

func (a ec2InstancesAnswer) keyNames() []string {
	var out []string
	for _, r := range a.Reservations {
		for _, i := range r.Instances {
			out = append(out, i.KeyName)
		}
	}
	return out
}

// awsPageValues is pick's values from every page's answer, decoded as T,
// in order, each once, blanks dropped. A page that does not parse fails.
func awsPageValues[T any](pages []AWSReadExchange, pick func(T) []string) ([]string, error) {
	var out []string
	for _, page := range pages {
		var answer T
		if err := xml.Unmarshal([]byte(page.Body), &answer); err != nil {
			return nil, fmt.Errorf("follow-up %s %s: the answer does not parse: %w", page.Action, page.Params, err)
		}
		out = awsAppendNew(out, pick(answer)...)
	}
	return out, nil
}

// awsAppendNew is out with each of values not already in it appended,
// trimmed, blanks dropped.
func awsAppendNew(out []string, values ...string) []string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
