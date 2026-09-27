---
kind: code
status: blocked
blocked_by: [fakeaws-sg-ip-permissions, fakeaws-provider-exact-pin, fakeaws-refuse-unknown-ami]
epic: fakeaws-step-one-surfaces
repo: fakeaws
depends_on: [fakeaws-sg-ip-permissions, fakeaws-provider-exact-pin, fakeaws-refuse-unknown-ami]
touches: ["handlers/ec2.go", "handlers/ec2_test.go", "examples/misconfigured/sg_ingress_inverted_port_range/ (new)", "examples/misconfigured/sg_ingress_port_out_of_range/ (new)", "examples/misconfigured/sg_ingress_unknown_protocol/ (new)", "examples/working/security_group_edge_rules/ (new)", "CHANGELOG.md"]
risk: high
---

# Item 11: security-group permissions refused with AWS's code and a case-specific message; positive controls still apply

Authorize{Ingress,Egress} validate each permission: IPv4 and IPv6 CIDRs; tcp and udp (by name or 6/17) ports 0-65535 with FromPort <= ToPort; icmp and icmpv6 type and code in range; protocol one of tcp, udp, icmp, icmpv6, -1 or all, or 0-255. The code is InvalidParameterValue (moto.md:267-271, confirmed per case in the EC2 API reference; a case AWS documents differently uses AWS's code). The message is AWS's (or moto's where AWS documents none), cited in the test. The provider forwards bad ports and protocols unvalidated (terraform-provider-aws vpc_security_group.go:148-150,168,181-184). Only CIDRs are checked at plan time (:140,:157), so malformed-CIDR cases are handler-only. New examples use the explicit endpoints block and 5.100.0.

**Done when:**
- Handler tests, each asserting the code and a message fragment unique to its case: IPv4 /33, 'bogus' CIDR, a bad IPv6 CIDR, tcp 80->70, tcp 70000, protocol 'bogus', protocol '256', protocol '6' with 80->70, icmp type 256, and one AuthorizeSecurityGroupEgress refusal. Each fails on today's code
- Positive controls return 200: -1 with 0.0.0.0/0, tcp 0-65535, icmp -1/-1, protocol '6' 80-80
- In the provider-smoke job, misconfigured/sg_ingress_inverted_port_range, sg_ingress_port_out_of_range and sg_ingress_unknown_protocol pass. Each expected.txt holds '<code>: <case message prefix>' as the provider prints it, so each proves which check fired
- working/security_group_edge_rules (the three positive controls) passes, and security_group_full_shape, vpc_network and update_security_group_rules stay green
