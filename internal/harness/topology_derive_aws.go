package harness

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// awsMockState is the slice of fakeaws /mock/state that the compute
// probe reads (fakeaws handlers/ec2.go gatherEC2StateReal).
type awsMockState struct {
	EC2 struct {
		Instances      []awsInstance `json:"instances"`
		SecurityGroups []struct {
			ID            string          `json:"id"`
			IPPermissions []awsPermission `json:"ip_permissions"`
		} `json:"security_groups"`
		Subnets []struct {
			ID    string `json:"id"`
			VPCID string `json:"vpc_id"`
		} `json:"subnets"`
		InternetGateways []struct {
			ID    string `json:"id"`
			VPCID string `json:"vpc_id"`
		} `json:"internet_gateways"`
		Routes []struct {
			RouteTableID         string `json:"route_table_id"`
			DestinationCidrBlock string `json:"destination_cidr_block"`
			GatewayID            string `json:"gateway_id"`
		} `json:"routes"`
		RouteTableAssociations []struct {
			RouteTableID string `json:"route_table_id"`
			SubnetID     string `json:"subnet_id"`
		} `json:"route_table_associations"`
	} `json:"ec2"`
}

type awsInstance struct {
	ID                  string   `json:"id"`
	SubnetID            string   `json:"subnet_id"`
	PublicIP            string   `json:"public_ip"`
	VPCSecurityGroupIDs []string `json:"vpc_security_group_ids"`
}

type awsPermission struct {
	IPProtocol string `json:"ip_protocol"`
	FromPort   int    `json:"from_port"`
	ToPort     int    `json:"to_port"`
	IPRanges   []struct {
		CidrIP string `json:"cidr_ip"`
	} `json:"ip_ranges"`
	IPv6Ranges []struct {
		CidrIPv6 string `json:"cidr_ipv6"`
	} `json:"ipv6_ranges"`
}

// awsIngress is what one security group admits on one tcp port.
type awsIngress struct {
	v4, v6 []string
}

// deriveTopologyAWS emits http_probe compute:<port> for every port named
// by a single-port tcp ingress rule with a CIDR source. A key is true
// only when some instance has that port open to 0.0.0.0/0 on one of its
// own groups, a public IPv4 address (the probe dials
// aws_instance.public_ip), and a subnet whose associated route table
// sends 0.0.0.0/0 to an internet gateway of the subnet's VPC. Each false
// key's diagnostic names the first failing condition; diagnostics
// ["compute"] covers probes on ports with no key. Connectivity is empty.
func deriveTopologyAWS(stateJSON []byte) ([]byte, map[string]string, error) {
	var state awsMockState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return nil, nil, fmt.Errorf("unmarshal aws state: %w", err)
	}
	probe := map[string]bool{}
	diagnostics := map[string]string{}

	ingress, notes := awsIngressByPort(&state)
	ports := make([]int, 0, len(ingress))
	for port := range ingress {
		ports = append(ports, port)
	}
	sort.Ints(ports)

	for _, port := range ports {
		key := httpProbeKey("compute", port)
		reason := "no instances"
		for _, inst := range state.EC2.Instances {
			r := awsInstanceUnreachable(&state, ingress[port], inst, port)
			if r == "" {
				reason = ""
				break
			}
			reason = r
		}
		probe[key] = reason == ""
		if reason != "" {
			diagnostics[key] = reason
		}
	}
	diagnostics["compute"] = awsComputeFallback(ports, notes)

	body, err := json.Marshal(map[string]any{
		"http_probe":   probe,
		"connectivity": map[string]bool{},
	})
	if err != nil {
		return nil, nil, err
	}
	return body, diagnostics, nil
}

// awsIngressByPort maps each single-port tcp port to what each security
// group admits on it. notes lists CIDR-sourced rules not derived.
func awsIngressByPort(state *awsMockState) (map[int]map[string]awsIngress, []string) {
	byPort := map[int]map[string]awsIngress{}
	var notes []string
	for _, sg := range state.EC2.SecurityGroups {
		for _, p := range sg.IPPermissions {
			var in awsIngress
			for _, r := range p.IPRanges {
				in.v4 = append(in.v4, r.CidrIP)
			}
			for _, r := range p.IPv6Ranges {
				in.v6 = append(in.v6, r.CidrIPv6)
			}
			if len(in.v4)+len(in.v6) == 0 {
				continue // group-to-group or prefix-list source: not internet-facing
			}
			// ponytail: port ranges and all-protocol rules are not derived;
			// expand them if a scenario ever probes through one.
			switch {
			case p.IPProtocol == "-1":
				notes = append(notes, fmt.Sprintf("protocol -1 (all traffic) rule on %s not derived", sg.ID))
				continue
			case p.IPProtocol != "tcp" && p.IPProtocol != "6":
				continue
			case p.FromPort != p.ToPort:
				notes = append(notes, fmt.Sprintf("tcp port range %d-%d on %s not derived", p.FromPort, p.ToPort, sg.ID))
				continue
			}
			if byPort[p.FromPort] == nil {
				byPort[p.FromPort] = map[string]awsIngress{}
			}
			prev := byPort[p.FromPort][sg.ID]
			byPort[p.FromPort][sg.ID] = awsIngress{v4: append(prev.v4, in.v4...), v6: append(prev.v6, in.v6...)}
		}
	}
	return byPort, notes
}

// awsInstanceUnreachable returns "" when the instance answers on port
// from the IPv4 internet, else the first failing condition.
func awsInstanceUnreachable(state *awsMockState, byGroup map[string]awsIngress, inst awsInstance, port int) string {
	id := inst.ID
	var v4, v6 []string
	for _, g := range inst.VPCSecurityGroupIDs {
		v4 = append(v4, byGroup[g].v4...)
		v6 = append(v6, byGroup[g].v6...)
	}
	switch {
	case slices.Contains(v4, "0.0.0.0/0"):
	case len(v4) > 0:
		return fmt.Sprintf("instance %s admits tcp %d only from %s, not 0.0.0.0/0", id, port, strings.Join(v4, ","))
	case len(v6) > 0:
		return fmt.Sprintf("instance %s admits tcp %d only from %s: IPv6-only ingress, and the probe dials the IPv4 public_ip", id, port, strings.Join(v6, ","))
	default:
		return fmt.Sprintf("no security group attached to instance %s admits tcp %d", id, port)
	}
	if inst.PublicIP == "" {
		return fmt.Sprintf("instance %s has no public IPv4 address", id)
	}
	return awsSubnetUnrouted(state, inst.SubnetID)
}

// awsSubnetUnrouted returns "" when the subnet's explicitly associated
// route table sends 0.0.0.0/0 to an internet gateway of the subnet's
// VPC. fakeaws exports no main-table flag, so an unassociated subnet
// counts as unrouted.
func awsSubnetUnrouted(state *awsMockState, subnetID string) string {
	vpcID := ""
	for _, s := range state.EC2.Subnets {
		if s.ID == subnetID {
			vpcID = s.VPCID
		}
	}
	tableID := ""
	for _, a := range state.EC2.RouteTableAssociations {
		if a.SubnetID == subnetID {
			tableID = a.RouteTableID
		}
	}
	if tableID == "" {
		return fmt.Sprintf("subnet %s has no route table association", subnetID)
	}
	for _, r := range state.EC2.Routes {
		if r.RouteTableID != tableID || r.DestinationCidrBlock != "0.0.0.0/0" {
			continue
		}
		for _, igw := range state.EC2.InternetGateways {
			if igw.ID == r.GatewayID && vpcID != "" && igw.VPCID == vpcID {
				return ""
			}
		}
	}
	return fmt.Sprintf("route table %s of subnet %s has no 0.0.0.0/0 route to an internet gateway of %s", tableID, subnetID, vpcID)
}

// awsComputeFallback explains a probe on a port with no compute key.
func awsComputeFallback(ports []int, notes []string) string {
	head := "no single-port tcp ingress rule with a cidr source"
	if len(ports) > 0 {
		strs := make([]string, len(ports))
		for i, p := range ports {
			strs[i] = strconv.Itoa(p)
		}
		head = "tcp ingress on port " + strings.Join(strs, ",")
	}
	return strings.Join(append([]string{head}, notes...), "; ")
}
